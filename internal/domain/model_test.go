package domain

import "testing"

func TestTimelineValidateReferencesAssetRanges(t *testing.T) {
	assets := map[string]MediaAsset{"a": {ID: "a", Path: "a.mp4", DurationUS: 10_000_000, Width: 1920, Height: 1080}}
	timeline := TimelineRevision{ID: "tl", Revision: 1, FPSNum: 30, FPSDen: 1, Width: 1920, Height: 1080, Items: []ClipItem{{ID: "c", AssetID: "a", SourceOutUS: 5_000_000, DurationFrames: 150}}}
	if err := timeline.Validate(assets); err != nil {
		t.Fatal(err)
	}
	timeline.Items[0].SourceOutUS = 11_000_000
	if err := timeline.Validate(assets); err == nil {
		t.Fatal("expected source range error")
	}
}
