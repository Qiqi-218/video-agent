package edit

import (
	"context"
	"errors"
	"fmt"

	"github.com/zylar06/video-agent/internal/domain"
	"github.com/zylar06/video-agent/internal/store"
)

type Engine struct {
	store store.TimelineStore
}

func NewEngine(s store.TimelineStore) *Engine { return &Engine{store: s} }

func (e *Engine) Apply(ctx context.Context, op domain.EditOperation) (domain.TimelineRevision, error) {
	if err := ctx.Err(); err != nil {
		return domain.TimelineRevision{}, err
	}
	if op.ID == "" || op.TimelineID == "" || op.BaseRevision < 1 {
		return domain.TimelineRevision{}, errors.New("operation requires id, timeline id and base revision")
	}
	if previous, ok := e.store.OperationResult(op.ID); ok {
		return previous, nil
	}
	current, err := e.store.Current(op.TimelineID)
	if err != nil {
		return domain.TimelineRevision{}, err
	}
	if current.Revision != op.BaseRevision {
		return domain.TimelineRevision{}, fmt.Errorf("revision conflict: current=%d base=%d", current.Revision, op.BaseRevision)
	}

	next := current.Clone()
	index := findClip(next.Items, op.TargetClipID)
	switch op.Kind {
	case domain.OpTrimClip:
		if index < 0 {
			return domain.TimelineRevision{}, errors.New("target clip not found")
		}
		if next.Items[index].Locked {
			return domain.TimelineRevision{}, errors.New("target clip is locked")
		}
		if op.DurationFrames < 1 || op.DurationFrames > next.Items[index].DurationFrames {
			return domain.TimelineRevision{}, errors.New("trim duration must be positive and no longer than current clip")
		}
		next.Items[index].DurationFrames = op.DurationFrames
	case domain.OpReplaceClip:
		if index < 0 {
			return domain.TimelineRevision{}, errors.New("target clip not found")
		}
		if next.Items[index].Locked {
			return domain.TimelineRevision{}, errors.New("target clip is locked")
		}
		if op.AssetID == "" || op.SourceOutUS <= op.SourceInUS {
			return domain.TimelineRevision{}, errors.New("replacement requires asset and source range")
		}
		next.Items[index].AssetID = op.AssetID
		next.Items[index].SourceInUS = op.SourceInUS
		next.Items[index].SourceOutUS = op.SourceOutUS
	case domain.OpInsertClip:
		if op.NewClipID == "" || op.AssetID == "" || op.SourceOutUS <= op.SourceInUS || op.DurationFrames < 1 {
			return domain.TimelineRevision{}, errors.New("insert requires new clip id, asset, source range and duration")
		}
		if op.Index < 0 || op.Index > len(next.Items) {
			return domain.TimelineRevision{}, errors.New("invalid insert index")
		}
		for _, item := range next.Items {
			if item.ID == op.NewClipID {
				return domain.TimelineRevision{}, errors.New("new clip id already exists")
			}
		}
		item := domain.ClipItem{ID: op.NewClipID, AssetID: op.AssetID, SourceInUS: op.SourceInUS, SourceOutUS: op.SourceOutUS, DurationFrames: op.DurationFrames}
		next.Items = append(next.Items, domain.ClipItem{})
		copy(next.Items[op.Index+1:], next.Items[op.Index:])
		next.Items[op.Index] = item
	case domain.OpDeleteClip:
		if index < 0 {
			return domain.TimelineRevision{}, errors.New("target clip not found")
		}
		if next.Items[index].Locked {
			return domain.TimelineRevision{}, errors.New("target clip is locked")
		}
		next.Items = append(next.Items[:index], next.Items[index+1:]...)
	case domain.OpMoveClip:
		if index < 0 || op.Index < 0 || op.Index >= len(next.Items) {
			return domain.TimelineRevision{}, errors.New("invalid move target")
		}
		item := next.Items[index]
		next.Items = append(next.Items[:index], next.Items[index+1:]...)
		if op.Index >= len(next.Items) {
			next.Items = append(next.Items, item)
		} else {
			next.Items = append(next.Items[:op.Index], append([]domain.ClipItem{item}, next.Items[op.Index:]...)...)
		}
	case domain.OpLockClip, domain.OpUnlockClip:
		if index < 0 {
			return domain.TimelineRevision{}, errors.New("target clip not found")
		}
		next.Items[index].Locked = op.Kind == domain.OpLockClip
	default:
		return domain.TimelineRevision{}, fmt.Errorf("unsupported operation %q", op.Kind)
	}

	next.ParentRevision = current.Revision
	next.Revision++
	if err := e.store.Save(next, op.ID); err != nil {
		return domain.TimelineRevision{}, err
	}
	return next, nil
}

func findClip(items []domain.ClipItem, id string) int {
	for i := range items {
		if items[i].ID == id {
			return i
		}
	}
	return -1
}
