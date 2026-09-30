package edit

import (
	"context"
	"strings"
	"testing"

	"github.com/zylar06/video-agent/internal/domain"
	"github.com/zylar06/video-agent/internal/store"
)

func fixture() (domain.TimelineRevision, *store.MemoryStore) {
	timeline := domain.TimelineRevision{ID: "tl", Revision: 1, FPSNum: 30, FPSDen: 1, Width: 1920, Height: 1080, Items: []domain.ClipItem{{ID: "c", AssetID: "a", SourceOutUS: 5_000_000, DurationFrames: 150}}}
	memory := store.NewMemoryStore()
	_ = memory.Open(timeline)
	return timeline, memory
}

func TestApplyTrimCreatesNewRevisionAndIsIdempotent(t *testing.T) {
	timeline, memory := fixture()
	engine := NewEngine(memory)
	op := domain.EditOperation{ID: "op-1", TimelineID: timeline.ID, BaseRevision: 1, Kind: domain.OpTrimClip, TargetClipID: "c", DurationFrames: 90}
	next, err := engine.Apply(context.Background(), op)
	if err != nil || next.Revision != 2 || next.Items[0].DurationFrames != 90 {
		t.Fatalf("unexpected first result: %#v, %v", next, err)
	}
	replay, err := engine.Apply(context.Background(), op)
	if err != nil || replay.Revision != 2 {
		t.Fatalf("unexpected replay: %#v, %v", replay, err)
	}
}

func TestApplyRejectsStaleRevisionAndLockedClip(t *testing.T) {
	timeline, memory := fixture()
	engine := NewEngine(memory)
	locked, err := engine.Apply(context.Background(), domain.EditOperation{ID: "lock", TimelineID: timeline.ID, BaseRevision: 1, Kind: domain.OpLockClip, TargetClipID: "c"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = engine.Apply(context.Background(), domain.EditOperation{ID: "stale", TimelineID: timeline.ID, BaseRevision: 1, Kind: domain.OpTrimClip, TargetClipID: "c", DurationFrames: 60})
	if err == nil || !strings.Contains(err.Error(), "revision conflict") {
		t.Fatalf("expected revision conflict, got %v", err)
	}
	_, err = engine.Apply(context.Background(), domain.EditOperation{ID: "trim-locked", TimelineID: timeline.ID, BaseRevision: locked.Revision, Kind: domain.OpTrimClip, TargetClipID: "c", DurationFrames: 60})
	if err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("expected locked error, got %v", err)
	}
}

func TestApplyInsertAddsClipAtIndex(t *testing.T) {
	timeline, memory := fixture()
	engine := NewEngine(memory)
	next, err := engine.Apply(context.Background(), domain.EditOperation{
		ID: "insert", TimelineID: timeline.ID, BaseRevision: 1, Kind: domain.OpInsertClip,
		NewClipID: "inserted", AssetID: "b", SourceInUS: 1_000_000, SourceOutUS: 2_000_000,
		DurationFrames: 30, Index: 0,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(next.Items) != 2 || next.Items[0].ID != "inserted" || next.Revision != 2 {
		t.Fatalf("unexpected inserted timeline: %#v", next)
	}
}
