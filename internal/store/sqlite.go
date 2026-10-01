// Package store persists projects, immutable asset records, revisions and render jobs.
package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/zylar06/video-agent/internal/domain"
	_ "modernc.org/sqlite"
)

var ErrConflict = errors.New("revision conflict")
var ErrNotFound = errors.New("not found")
var ErrOperationReuse = errors.New("operation id reused with different payload")

type Store struct {
	db  *sql.DB
	Dir string
}

func Open(dir string) (*Store, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: filepath.ToSlash(filepath.Join(dir, "video-agent.db"))}
	db, err := sql.Open("sqlite", u.String()+"?_txlock=immediate&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	fail := func(e error) (*Store, error) { db.Close(); return nil, e }
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fail(err)
	}
	if version > 1 {
		return fail(errors.New("database is newer than this application"))
	}
	_, err = db.Exec(`
CREATE TABLE IF NOT EXISTS projects(id TEXT PRIMARY KEY, body BLOB NOT NULL);
CREATE TABLE IF NOT EXISTS assets(project_id TEXT NOT NULL REFERENCES projects(id), id TEXT NOT NULL, body BLOB NOT NULL, PRIMARY KEY(project_id,id));
CREATE TABLE IF NOT EXISTS timelines(id TEXT PRIMARY KEY, project_id TEXT NOT NULL REFERENCES projects(id), revision INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS revisions(timeline_id TEXT NOT NULL REFERENCES timelines(id), revision INTEGER NOT NULL, body BLOB NOT NULL, PRIMARY KEY(timeline_id,revision));
CREATE TABLE IF NOT EXISTS operations(timeline_id TEXT NOT NULL, id TEXT NOT NULL, payload BLOB NOT NULL, revision INTEGER NOT NULL, PRIMARY KEY(timeline_id,id), FOREIGN KEY(timeline_id,revision) REFERENCES revisions(timeline_id,revision));
CREATE TABLE IF NOT EXISTS jobs(id TEXT PRIMARY KEY, timeline_id TEXT NOT NULL, revision INTEGER NOT NULL, status TEXT NOT NULL, body BLOB NOT NULL, FOREIGN KEY(timeline_id,revision) REFERENCES revisions(timeline_id,revision));
PRAGMA user_version=1;`)
	if err != nil {
		return fail(err)
	}
	return &Store{db: db, Dir: dir}, nil
}
func (s *Store) Close() error { return s.db.Close() }
func decode(row *sql.Row, out any) error {
	var b []byte
	if err := row.Scan(&b); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return err
	}
	return json.Unmarshal(b, out)
}

func (s *Store) CreateProject(p domain.Project) error {
	if p.ID == "" || p.Name == "" {
		return errors.New("project requires id and name")
	}
	b, err := json.Marshal(p)
	if err != nil {
		return err
	}
	_, err = s.db.Exec("INSERT INTO projects VALUES(?,?)", p.ID, b)
	return err
}
func (s *Store) Project(id string) (p domain.Project, err error) {
	err = decode(s.db.QueryRow("SELECT body FROM projects WHERE id=?", id), &p)
	return
}
func (s *Store) PutAsset(a domain.MediaAsset) (domain.MediaAsset, error) {
	if err := a.Validate(); err != nil {
		return a, err
	}
	b, err := json.Marshal(a)
	if err != nil {
		return a, err
	}
	_, err = s.db.Exec("INSERT INTO assets VALUES(?,?,?) ON CONFLICT(project_id,id) DO NOTHING", a.ProjectID, a.ID, b)
	if err != nil {
		return a, err
	}
	return s.Asset(a.ProjectID, a.ID)
}
func (s *Store) Asset(project, id string) (a domain.MediaAsset, err error) {
	err = decode(s.db.QueryRow("SELECT body FROM assets WHERE project_id=? AND id=?", project, id), &a)
	return
}
func (s *Store) Assets(project string) (map[string]domain.MediaAsset, error) {
	rows, err := s.db.Query("SELECT body FROM assets WHERE project_id=? ORDER BY id", project)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	assets := map[string]domain.MediaAsset{}
	for rows.Next() {
		var b []byte
		var a domain.MediaAsset
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &a); err != nil {
			return nil, err
		}
		assets[a.ID] = a
	}
	return assets, rows.Err()
}
func (s *Store) CreateTimeline(t domain.TimelineRevision) error {
	if t.Revision != 1 {
		return errors.New("new timeline must start at revision 1")
	}
	assets, err := s.Assets(t.ProjectID)
	if err != nil {
		return err
	}
	if err = t.Validate(assets); err != nil {
		return err
	}
	b, err := json.Marshal(t)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("INSERT INTO timelines VALUES(?,?,?)", t.ID, t.ProjectID, 1); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO revisions VALUES(?,?,?)", t.ID, 1, b); err != nil {
		return err
	}
	return tx.Commit()
}
func (s *Store) Current(id string) (t domain.TimelineRevision, err error) {
	err = decode(s.db.QueryRow("SELECT r.body FROM revisions r JOIN timelines t ON r.timeline_id=t.id AND r.revision=t.revision WHERE t.id=?", id), &t)
	return
}
func (s *Store) Revision(id string, revision int) (t domain.TimelineRevision, err error) {
	err = decode(s.db.QueryRow("SELECT body FROM revisions WHERE timeline_id=? AND revision=?", id, revision), &t)
	return
}
func (s *Store) History(id string) ([]domain.TimelineRevision, error) {
	rows, err := s.db.Query("SELECT body FROM revisions WHERE timeline_id=? ORDER BY revision", id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.TimelineRevision{}
	for rows.Next() {
		var b []byte
		var t domain.TimelineRevision
		if err = rows.Scan(&b); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(b, &t); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

type queryer interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func operationResult(ctx context.Context, q queryer, op domain.EditOperation) (t domain.TimelineRevision, found bool, err error) {
	var payload, body []byte
	err = q.QueryRowContext(ctx, `SELECT o.payload,r.body FROM operations o JOIN revisions r ON r.timeline_id=o.timeline_id AND r.revision=o.revision WHERE o.timeline_id=? AND o.id=?`, op.TimelineID, op.ID).Scan(&payload, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return t, false, nil
	}
	if err != nil {
		return t, false, err
	}
	want, _ := json.Marshal(op)
	if string(payload) != string(want) {
		return t, false, ErrOperationReuse
	}
	err = json.Unmarshal(body, &t)
	return t, true, err
}
func (s *Store) OperationResult(ctx context.Context, op domain.EditOperation) (domain.TimelineRevision, bool, error) {
	return operationResult(ctx, s.db, op)
}

// Save atomically checks the base revision and records BOTH the snapshot and operation.
func (s *Store) Save(ctx context.Context, t domain.TimelineRevision, op domain.EditOperation) (domain.TimelineRevision, error) {
	assets, err := s.Assets(t.ProjectID)
	if err != nil {
		return t, err
	}
	if err = t.Validate(assets); err != nil {
		return t, err
	}
	if t.ID != op.TimelineID || t.Revision != op.BaseRevision+1 {
		return t, ErrConflict
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return t, err
	}
	defer tx.Rollback()
	if prev, ok, e := operationResult(ctx, tx, op); e != nil || ok {
		return prev, e
	}
	r, err := tx.ExecContext(ctx, "UPDATE timelines SET revision=? WHERE id=? AND project_id=? AND revision=?", t.Revision, t.ID, t.ProjectID, op.BaseRevision)
	if err != nil {
		return t, err
	}
	n, err := r.RowsAffected()
	if err != nil {
		return t, err
	}
	if n != 1 {
		return t, ErrConflict
	}
	b, _ := json.Marshal(t)
	payload, _ := json.Marshal(op)
	if _, err = tx.ExecContext(ctx, "INSERT INTO revisions VALUES(?,?,?)", t.ID, t.Revision, b); err != nil {
		return t, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO operations VALUES(?,?,?,?)", t.ID, op.ID, payload, t.Revision); err != nil {
		return t, err
	}
	return t, tx.Commit()
}

func (s *Store) CreateJob(j domain.RenderJob) error {
	if j.ID == "" || j.Status != "running" || (j.Kind != "export" && j.Kind != "preview") {
		return errors.New("invalid render job")
	}
	j.UpdatedAt = time.Now().UTC()
	b, _ := json.Marshal(j)
	_, err := s.db.Exec("INSERT INTO jobs VALUES(?,?,?,?,?)", j.ID, j.TimelineID, j.Revision, j.Status, b)
	return err
}
func (s *Store) FinishJob(j domain.RenderJob) error {
	if j.Status != "completed" && j.Status != "failed" && j.Status != "cancelled" {
		return errors.New("invalid terminal job status")
	}
	if j.Status == "completed" && (j.Validation == nil || !j.Validation.ProbePassed || !j.Validation.DecodePassed) {
		return errors.New("job requires verified output")
	}
	j.UpdatedAt = time.Now().UTC()
	b, _ := json.Marshal(j)
	r, err := s.db.Exec("UPDATE jobs SET status=?,body=? WHERE id=? AND status='running'", j.Status, b, j.ID)
	if err != nil {
		return err
	}
	n, _ := r.RowsAffected()
	if n != 1 {
		return fmt.Errorf("job state conflict: %s", j.ID)
	}
	return nil
}
func (s *Store) Job(id string) (j domain.RenderJob, err error) {
	err = decode(s.db.QueryRow("SELECT body FROM jobs WHERE id=?", id), &j)
	return
}
