package agent_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/zylar06/video-agent/internal/agent"
	"github.com/zylar06/video-agent/internal/app"
	"github.com/zylar06/video-agent/internal/domain"
)

func call(t *testing.T, s *agent.Service, name string, in any) agent.Envelope {
	t.Helper()
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	return s.Call(context.Background(), name, b)
}

func TestEvidenceSearchProposalAndRevisionGuard(t *testing.T) {
	a, err := app.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	if err := a.Store.CreateProject(domain.Project{ID: "p", Name: "P3"}); err != nil {
		t.Fatal(err)
	}
	asset, err := a.Store.PutAsset(domain.MediaAsset{ID: "a", ProjectID: "p", Path: "/fixture.mp4", ContentHash: "hash", DurationUS: 30_000_000, Width: 1280, Height: 720, FPS: "30/1", Status: "ready"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.CreateTimeline(domain.TimelineRevision{ID: "t", ProjectID: "p", FPSNum: 30, FPSDen: 1, Width: 1280, Height: 720}); err != nil {
		t.Fatal(err)
	}
	s := agent.NewService(a)
	missing := call(t, s, "analyze", map[string]any{"project_id": "p", "asset_id": asset.ID})
	if missing.OK || missing.Error.Code != "model_unavailable" {
		t.Fatalf("expected unavailable analyzer: %+v", missing)
	}
	e := call(t, s, "evidence_add", domain.Evidence{ID: "ev-1", ProjectID: "p", AssetID: asset.ID, StartUS: 2_000_000, EndUS: 5_000_000, Transcript: "绝杀进球 全场欢呼", AnalyzerVersion: "srt-v1", Provider: "fixture"})
	if !e.OK {
		t.Fatalf("evidence: %+v", e)
	}
	analyzed := call(t, s, "analyze", map[string]any{"project_id": "p", "asset_id": asset.ID})
	if !analyzed.OK {
		t.Fatalf("analyze indexed evidence: %+v", analyzed)
	}
	match := call(t, s, "search", map[string]any{"project_id": "p", "query": "进球", "limit": 3})
	if !match.OK {
		t.Fatalf("search: %+v", match)
	}
	noMatch := call(t, s, "search", map[string]any{"project_id": "p", "query": "颁奖", "limit": 3})
	if noMatch.OK || noMatch.Error.Code != "no_match" {
		t.Fatalf("expected no_match, got %+v", noMatch)
	}
	p := call(t, s, "proposal_create", map[string]any{"timeline_id": "t", "query": "绝杀", "limit": 1})
	if !p.OK {
		t.Fatalf("proposal: %+v", p)
	}
	b, _ := json.Marshal(p.Result)
	var proposal domain.EditProposal
	if err := json.Unmarshal(b, &proposal); err != nil {
		t.Fatal(err)
	}
	if len(proposal.Operations) != 1 || proposal.Operations[0].BaseRevision != 1 {
		t.Fatalf("bad proposal: %+v", proposal)
	}
	applied := call(t, s, "edit_apply", proposal.Operations[0])
	if !applied.OK {
		t.Fatalf("apply: %+v", applied)
	}
	stale := proposal.Operations[0]
	stale.ID = "stale-op"
	conflict := call(t, s, "edit_apply", stale)
	if conflict.OK || conflict.Error.Code != "revision_conflict" {
		t.Fatalf("expected conflict: %+v", conflict)
	}
}
