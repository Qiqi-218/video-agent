package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/zylar06/video-agent/internal/domain"
	"github.com/zylar06/video-agent/internal/edit"
	"github.com/zylar06/video-agent/internal/render"
	"github.com/zylar06/video-agent/internal/store"
)

func main() {
	if len(os.Args) != 2 || os.Args[1] != "demo" {
		fmt.Fprintln(os.Stderr, "usage: video-agent demo")
		os.Exit(2)
	}

	asset := domain.MediaAsset{ID: "asset-demo", Path: "demo.mp4", DurationUS: 10_000_000, Width: 1920, Height: 1080, Status: "ready"}
	timeline := domain.TimelineRevision{
		ID: "timeline-demo", Revision: 1, FPSNum: 30, FPSDen: 1, Width: 1920, Height: 1080,
		Items: []domain.ClipItem{{ID: "clip-demo", AssetID: asset.ID, SourceInUS: 0, SourceOutUS: 5_000_000, StartFrame: 0, DurationFrames: 150}},
	}

	mem := store.NewMemoryStore()
	if err := mem.Open(timeline); err != nil {
		fatal(err)
	}
	engine := edit.NewEngine(mem)
	next, err := engine.Apply(context.Background(), domain.EditOperation{
		ID: "op-demo-trim", TimelineID: timeline.ID, BaseRevision: 1, Kind: domain.OpTrimClip,
		TargetClipID: "clip-demo", DurationFrames: 90,
	})
	if err != nil {
		fatal(err)
	}

	plan, err := render.Compile(next, []domain.MediaAsset{asset})
	if err != nil {
		fatal(err)
	}
	output := map[string]any{"timeline": next, "render_plan": plan}
	encoded, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		fatal(err)
	}
	fmt.Println(string(encoded))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
