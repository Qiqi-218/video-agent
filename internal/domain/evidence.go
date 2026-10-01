package domain

import "errors"

// Evidence is a bounded, source-addressable observation about an imported
// asset. P3 accepts evidence produced by an external analyzer (for example an
// SRT importer); P2 owns automatic ASR and visual providers.
type Evidence struct {
	ID               string   `json:"id"`
	ProjectID        string   `json:"project_id"`
	AssetID          string   `json:"asset_id"`
	StartUS          int64    `json:"start_us"`
	EndUS            int64    `json:"end_us"`
	AssetContentHash string   `json:"asset_content_hash"`
	Transcript       string   `json:"transcript,omitempty"`
	VisualSummary    string   `json:"visual_summary,omitempty"`
	FrameRefs        []string `json:"frame_refs,omitempty"`
	AnalyzerVersion  string   `json:"analyzer_version,omitempty"`
	Provider         string   `json:"provider,omitempty"`
	CacheKey         string   `json:"cache_key,omitempty"`
}

func (e Evidence) Validate(asset MediaAsset) error {
	if e.ID == "" || e.ProjectID == "" || e.AssetID == "" || e.ProjectID != asset.ProjectID || e.AssetID != asset.ID || e.StartUS < 0 || e.EndUS <= e.StartUS || e.EndUS > asset.DurationUS {
		return errors.New("invalid evidence source range")
	}
	if e.Transcript == "" && e.VisualSummary == "" && len(e.FrameRefs) == 0 {
		return errors.New("evidence requires transcript, visual_summary, or frame_refs")
	}
	if e.AssetContentHash != asset.ContentHash {
		return errors.New("evidence asset_content_hash does not match imported asset")
	}
	return nil
}

type EditProposal struct {
	ID           string          `json:"id"`
	TimelineID   string          `json:"timeline_id"`
	BaseRevision int             `json:"base_revision"`
	Query        string          `json:"query"`
	EvidenceIDs  []string        `json:"evidence_ids"`
	Operations   []EditOperation `json:"operations"`
}
