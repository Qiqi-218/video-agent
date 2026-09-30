package render

import (
	"fmt"

	"github.com/zylar06/video-agent/internal/domain"
)

type InputRange struct {
	AssetID        string `json:"asset_id"`
	Path           string `json:"path"`
	SourceInUS     int64  `json:"source_in_us"`
	SourceOutUS    int64  `json:"source_out_us"`
	StartFrame     int    `json:"start_frame"`
	DurationFrames int    `json:"duration_frames"`
}

type Plan struct {
	TimelineID string       `json:"timeline_id"`
	Revision   int          `json:"revision"`
	Width      int          `json:"width"`
	Height     int          `json:"height"`
	FPSNum     int          `json:"fps_num"`
	FPSDen     int          `json:"fps_den"`
	Inputs     []InputRange `json:"inputs"`
}

func Compile(t domain.TimelineRevision, assets []domain.MediaAsset) (Plan, error) {
	byID := make(map[string]domain.MediaAsset, len(assets))
	for _, asset := range assets {
		byID[asset.ID] = asset
	}
	if err := t.Validate(byID); err != nil {
		return Plan{}, err
	}
	plan := Plan{TimelineID: t.ID, Revision: t.Revision, Width: t.Width, Height: t.Height, FPSNum: t.FPSNum, FPSDen: t.FPSDen}
	for _, item := range t.Items {
		asset := byID[item.AssetID]
		plan.Inputs = append(plan.Inputs, InputRange{AssetID: item.AssetID, Path: asset.Path, SourceInUS: item.SourceInUS, SourceOutUS: item.SourceOutUS, StartFrame: item.StartFrame, DurationFrames: item.DurationFrames})
	}
	return plan, nil
}

func (p Plan) Validate() error {
	if p.TimelineID == "" || p.Revision < 1 || p.Width <= 0 || p.Height <= 0 || p.FPSNum <= 0 || p.FPSDen <= 0 {
		return fmt.Errorf("invalid render plan")
	}
	for _, input := range p.Inputs {
		if input.AssetID == "" || input.Path == "" || input.SourceOutUS <= input.SourceInUS || input.DurationFrames <= 0 {
			return fmt.Errorf("invalid render input %q", input.AssetID)
		}
	}
	return nil
}
