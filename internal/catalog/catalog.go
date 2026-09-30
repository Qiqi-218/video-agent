package catalog

import "context"

type Evidence struct {
	ID            string   `json:"id"`
	AssetID       string   `json:"asset_id"`
	StartUS       int64    `json:"start_us"`
	EndUS         int64    `json:"end_us"`
	Transcript    string   `json:"transcript,omitempty"`
	VisualSummary string   `json:"visual_summary,omitempty"`
	FrameRefs     []string `json:"frame_refs,omitempty"`
}

type SearchRequest struct {
	Query    string
	AssetIDs []string
	Limit    int
}

type SearchResult struct {
	Evidence Evidence `json:"evidence"`
	Score    float64  `json:"score"`
	Reason   string   `json:"reason"`
}

type Catalog interface {
	Analyze(context.Context, string) ([]Evidence, error)
	Search(context.Context, SearchRequest) ([]SearchResult, error)
}
