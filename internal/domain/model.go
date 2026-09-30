package domain

import (
	"errors"
	"fmt"
)

type MediaAsset struct {
	ID          string `json:"id"`
	Path        string `json:"path"`
	ContentHash string `json:"content_hash,omitempty"`
	DurationUS  int64  `json:"duration_us"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
	HasAudio    bool   `json:"has_audio"`
	Status      string `json:"status"`
}

type ClipItem struct {
	ID             string   `json:"id"`
	AssetID        string   `json:"asset_id"`
	SourceInUS     int64    `json:"source_in_us"`
	SourceOutUS    int64    `json:"source_out_us"`
	StartFrame     int      `json:"start_frame"`
	DurationFrames int      `json:"duration_frames"`
	Locked         bool     `json:"locked"`
	EvidenceIDs    []string `json:"evidence_ids,omitempty"`
}

type TimelineRevision struct {
	ID             string     `json:"id"`
	Revision       int        `json:"revision"`
	ParentRevision int        `json:"parent_revision"`
	FPSNum         int        `json:"fps_num"`
	FPSDen         int        `json:"fps_den"`
	Width          int        `json:"width"`
	Height         int        `json:"height"`
	Items          []ClipItem `json:"items"`
}

type OperationKind string

const (
	OpTrimClip    OperationKind = "trim_clip"
	OpReplaceClip OperationKind = "replace_clip"
	OpDeleteClip  OperationKind = "delete_clip"
	OpInsertClip  OperationKind = "insert_clip"
	OpMoveClip    OperationKind = "move_clip"
	OpLockClip    OperationKind = "lock_clip"
	OpUnlockClip  OperationKind = "unlock_clip"
)

type EditOperation struct {
	ID             string        `json:"id"`
	TimelineID     string        `json:"timeline_id"`
	BaseRevision   int           `json:"base_revision"`
	Kind           OperationKind `json:"kind"`
	TargetClipID   string        `json:"target_clip_id,omitempty"`
	NewClipID      string        `json:"new_clip_id,omitempty"`
	AssetID        string        `json:"asset_id,omitempty"`
	SourceInUS     int64         `json:"source_in_us,omitempty"`
	SourceOutUS    int64         `json:"source_out_us,omitempty"`
	DurationFrames int           `json:"duration_frames,omitempty"`
	Index          int           `json:"index,omitempty"`
}

func (a MediaAsset) Validate() error {
	if a.ID == "" || a.Path == "" || a.DurationUS <= 0 || a.Width <= 0 || a.Height <= 0 {
		return errors.New("asset requires id, path, positive duration and dimensions")
	}
	return nil
}

func (t TimelineRevision) Validate(assets map[string]MediaAsset) error {
	if t.ID == "" || t.Revision < 1 || t.FPSNum <= 0 || t.FPSDen <= 0 || t.Width <= 0 || t.Height <= 0 {
		return errors.New("invalid timeline header")
	}
	seen := make(map[string]bool, len(t.Items))
	for _, item := range t.Items {
		if item.ID == "" || seen[item.ID] || item.AssetID == "" || item.StartFrame < 0 || item.DurationFrames <= 0 {
			return fmt.Errorf("invalid clip %q", item.ID)
		}
		asset, ok := assets[item.AssetID]
		if !ok {
			return fmt.Errorf("clip %q references unknown asset %q", item.ID, item.AssetID)
		}
		if item.SourceInUS < 0 || item.SourceOutUS <= item.SourceInUS || item.SourceOutUS > asset.DurationUS {
			return fmt.Errorf("clip %q has invalid source range", item.ID)
		}
		seen[item.ID] = true
	}
	return nil
}

func (t TimelineRevision) Clone() TimelineRevision {
	clone := t
	clone.Items = append([]ClipItem(nil), t.Items...)
	for i := range clone.Items {
		clone.Items[i].EvidenceIDs = append([]string(nil), t.Items[i].EvidenceIDs...)
	}
	return clone
}
