package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/zylar06/video-agent/internal/app"
	"github.com/zylar06/video-agent/internal/catalog"
	"github.com/zylar06/video-agent/internal/domain"
	"github.com/zylar06/video-agent/internal/edit"
	"github.com/zylar06/video-agent/internal/store"
)

const APIVersion = "v1"

type APIError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Envelope struct {
	APIVersion string    `json:"api_version"`
	OK         bool      `json:"ok"`
	Result     any       `json:"result,omitempty"`
	Error      *APIError `json:"error,omitempty"`
}

// Service is the constrained P3 boundary used by both the JSON CLI and HTTP.
// It deliberately exposes typed actions rather than raw database or filesystem
// access, so an external Code Agent cannot bypass revisions or asset ownership.
type Service struct{ App *app.App }

func NewService(a *app.App) *Service { return &Service{App: a} }

func (s *Service) Names() []string {
	return []string{"analyze", "assets_import", "assets_list", "edit_apply", "evidence_add", "jobs_cancel", "jobs_get", "jobs_list", "project_create", "project_get", "proposal_create", "render_submit", "search", "timeline_create", "timeline_get", "timeline_history"}
}

func (s *Service) Call(ctx context.Context, name string, raw json.RawMessage) Envelope {
	result, err := s.call(ctx, name, raw)
	if err != nil {
		return Envelope{APIVersion: APIVersion, OK: false, Error: classify(err)}
	}
	return Envelope{APIVersion: APIVersion, OK: true, Result: result}
}

func decode(raw json.RawMessage, out any) error {
	if len(raw) == 0 {
		raw = []byte("{}")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return fmt.Errorf("invalid input: %w", err)
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("invalid input: expected exactly one JSON object")
	}
	return nil
}

func (s *Service) call(ctx context.Context, name string, raw json.RawMessage) (any, error) {
	switch name {
	case "project_create":
		var in domain.Project
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return in, s.App.Store.CreateProject(in)
	case "project_get":
		var in struct {
			ID string `json:"id"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return s.App.Store.Project(in.ID)
	case "assets_import":
		var in struct {
			ProjectID string `json:"project_id"`
			Path      string `json:"path"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return s.App.Tools.Import(ctx, s.App.Store, in.ProjectID, in.Path)
	case "assets_list":
		var in struct {
			ProjectID string `json:"project_id"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if _, err := s.App.Store.Project(in.ProjectID); err != nil {
			return nil, err
		}
		return s.App.Store.Assets(in.ProjectID)
	case "timeline_create":
		var in domain.TimelineRevision
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return s.App.CreateTimeline(in)
	case "timeline_get":
		var in struct {
			ID       string `json:"id"`
			Revision int    `json:"revision,omitempty"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if in.Revision == 0 {
			return s.App.Store.Current(in.ID)
		}
		return s.App.Store.Revision(in.ID, in.Revision)
	case "timeline_history":
		var in struct {
			ID string `json:"id"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return s.App.Store.History(in.ID)
	case "evidence_add":
		var in domain.Evidence
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return s.App.Store.PutEvidence(in)
	case "analyze":
		var in struct {
			ProjectID string `json:"project_id"`
			AssetID   string `json:"asset_id"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		if _, err := s.App.Store.Asset(in.ProjectID, in.AssetID); err != nil {
			return nil, err
		}
		evidence, err := s.App.Store.Evidence(in.ProjectID, []string{in.AssetID})
		if err != nil {
			return nil, err
		}
		if len(evidence) == 0 {
			return nil, errors.New("model provider unavailable: no indexed evidence; add evidence or configure a P2 analyzer")
		}
		return map[string]any{"status": "completed", "project_id": in.ProjectID, "asset_id": in.AssetID, "evidence": evidence}, nil
	case "search":
		var in catalog.SearchRequest
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		result, err := s.App.Store.SearchEvidence(in.ProjectID, in.Query, in.AssetIDs, in.Limit)
		if err != nil {
			return nil, err
		}
		if len(result) == 0 {
			return nil, errNoMatch(in.Query)
		}
		return result, nil
	case "proposal_create":
		var in struct {
			TimelineID string `json:"timeline_id"`
			Query      string `json:"query"`
			Limit      int    `json:"limit,omitempty"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return s.propose(in.TimelineID, in.Query, in.Limit)
	case "edit_apply":
		var in domain.EditOperation
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return edit.NewEngine(s.App.Store).Apply(ctx, in)
	case "render_submit":
		var in struct {
			TimelineID string `json:"timeline_id"`
			Revision   int    `json:"revision,omitempty"`
			Preview    bool   `json:"preview,omitempty"`
			Filename   string `json:"filename,omitempty"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return s.App.SubmitRender(in.TimelineID, in.Revision, in.Preview, in.Filename)
	case "jobs_get":
		var in struct {
			ID string `json:"id"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return s.App.Store.Job(in.ID)
	case "jobs_list":
		var in struct{}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return s.App.Store.Jobs()
	case "jobs_cancel":
		var in struct {
			ID string `json:"id"`
		}
		if err := decode(raw, &in); err != nil {
			return nil, err
		}
		return s.App.CancelJob(in.ID)
	default:
		return nil, fmt.Errorf("unknown tool %q", name)
	}
}

func (s *Service) propose(timelineID, query string, limit int) (domain.EditProposal, error) {
	if limit == 0 {
		limit = 3
	}
	t, err := s.App.Store.Current(timelineID)
	if err != nil {
		return domain.EditProposal{}, err
	}
	results, err := s.App.Store.SearchEvidence(t.ProjectID, query, nil, limit)
	if err != nil {
		return domain.EditProposal{}, err
	}
	if len(results) == 0 {
		return domain.EditProposal{}, errNoMatch(query)
	}
	p := domain.EditProposal{ID: "proposal-" + app.ID(), TimelineID: t.ID, BaseRevision: t.Revision, Query: query}
	for i, result := range results {
		e := result.Evidence
		p.EvidenceIDs = append(p.EvidenceIDs, e.ID)
		p.Operations = append(p.Operations, domain.EditOperation{ID: fmt.Sprintf("%s-%02d", p.ID, i+1), TimelineID: t.ID, BaseRevision: t.Revision + i, Kind: domain.OpInsertClip, NewClipID: fmt.Sprintf("candidate-%s-%02d", p.ID[len("proposal-"):], i+1), AssetID: e.AssetID, SourceInUS: e.StartUS, SourceOutUS: e.EndUS, Index: len(t.Items) + i})
	}
	return p, nil
}

type noMatchError struct{ query string }

func (e noMatchError) Error() string { return "no source evidence matches query: " + e.query }
func errNoMatch(query string) error  { return noMatchError{query: query} }

func classify(err error) *APIError {
	code := "invalid_request"
	switch {
	case errors.As(err, new(noMatchError)):
		code = "no_match"
	case errors.Is(err, store.ErrNotFound):
		code = "not_found"
	case errors.Is(err, store.ErrConflict):
		code = "revision_conflict"
	case errors.Is(err, context.Canceled):
		code = "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		code = "timeout"
	case strings.Contains(err.Error(), "model"):
		code = "model_unavailable"
	case strings.Contains(err.Error(), "state conflict"):
		code = "revision_conflict"
	}
	return &APIError{Code: code, Message: err.Error()}
}
