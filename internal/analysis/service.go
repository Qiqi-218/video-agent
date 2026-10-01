package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/zylar06/video-agent/internal/analysis/asr"
	"github.com/zylar06/video-agent/internal/analysis/provider"
	"github.com/zylar06/video-agent/internal/analysis/visual"
	"github.com/zylar06/video-agent/internal/domain"
	"github.com/zylar06/video-agent/internal/media"
	"github.com/zylar06/video-agent/internal/store"
)

type Request struct {
	ProjectID       string         `json:"project_id"`
	AssetID         string         `json:"asset_id"`
	SubtitlePath    string         `json:"subtitle_path,omitempty"`
	AnalyzerVersion string         `json:"analyzer_version"`
	Provider        string         `json:"provider"`
	Parameters      map[string]any `json:"parameters,omitempty"`
	Visual          bool           `json:"visual,omitempty"`
}

type Result struct {
	Run      domain.AnalysisRun `json:"run"`
	Evidence []domain.Evidence  `json:"evidence"`
}

type ASRProvider interface {
	Transcribe(context.Context, domain.MediaAsset) ([]asr.Cue, error)
}
type VisionProvider interface {
	Describe(context.Context, []domain.Evidence) (map[string]string, error)
}

type Service struct {
	Store  *store.Store
	Tools  media.Tools
	ASR    ASRProvider
	Vision VisionProvider
}

func New(s *store.Store, tools media.Tools) *Service {
	asrConfig := provider.ConfigFromEnvAliases("VIDEO_AGENT_ASR", "AUTOCLIP_ASR")
	visionConfig := provider.ConfigFromEnvAliases("VIDEO_AGENT_VISION", "AUTOCLIP_VISION")
	var asrProvider ASRProvider
	if asrConfig.BaseURL != "" && asrConfig.Model != "" && asrConfig.APIKey != "" {
		asrProvider = provider.OpenAITranscriber{Config: asrConfig}
	}
	var visionProvider VisionProvider
	if visionConfig.BaseURL != "" && visionConfig.Model != "" && visionConfig.APIKey != "" {
		visionProvider = provider.OpenAIVision{Config: visionConfig}
	}
	return &Service{Store: s, Tools: tools, ASR: asrProvider, Vision: visionProvider}
}

func NewWithProviders(s *store.Store, tools media.Tools, asrProvider ASRProvider, visionProvider VisionProvider) *Service {
	return &Service{Store: s, Tools: tools, ASR: asrProvider, Vision: visionProvider}
}

func CacheKey(assetHash, version, provider string, parameters map[string]any) (string, error) {
	b, err := json.Marshal(struct {
		Asset, Version, Provider string
		Parameters               map[string]any
	}{assetHash, version, provider, parameters})
	if err != nil {
		return "", err
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:]), nil
}

func (s *Service) Analyze(ctx context.Context, req Request) (Result, error) {
	asset, err := s.Store.Asset(req.ProjectID, req.AssetID)
	if err != nil {
		return Result{}, err
	}
	if req.AnalyzerVersion == "" {
		req.AnalyzerVersion = "p2-v1"
	}
	if req.Provider == "" {
		req.Provider = "subtitle"
	}
	parameters := map[string]any{}
	for k, v := range req.Parameters {
		parameters[k] = v
	}
	parameters["_visual"] = req.Visual
	if req.SubtitlePath != "" {
		b, hashErr := os.ReadFile(req.SubtitlePath)
		if hashErr != nil {
			return Result{}, hashErr
		}
		h := sha256.Sum256(b)
		parameters["_subtitle_content_hash"] = hex.EncodeToString(h[:])
	}
	key, err := CacheKey(asset.ContentHash, req.AnalyzerVersion, req.Provider, parameters)
	if err != nil {
		return Result{}, err
	}
	if old, err := s.Store.AnalysisRun(req.ProjectID, req.AssetID, key); err == nil && old.Status == "completed" {
		return s.resultFromRun(old)
	}
	run := domain.AnalysisRun{ProjectID: req.ProjectID, AssetID: req.AssetID, AssetContentHash: asset.ContentHash, CacheKey: key, AnalyzerVersion: req.AnalyzerVersion, Provider: req.Provider, Parameters: parameters, Status: "running", Stages: map[string]string{}}
	if _, err = s.Store.PutAnalysisRun(run); err != nil {
		return Result{}, err
	}
	all := []domain.Evidence{}
	if req.SubtitlePath != "" {
		run.Stages["subtitle"] = "running"
		_, _ = s.Store.PutAnalysisRun(run)
		cues, parseErr := asr.ParseFile(ctx, req.SubtitlePath)
		if parseErr != nil {
			return s.fail(run, "subtitle", parseErr)
		}
		for _, e := range asr.ToEvidence(req.ProjectID, asset, cues, req.Provider, req.AnalyzerVersion) {
			if _, err = s.Store.PutEvidence(e); err != nil {
				return s.fail(run, "subtitle", err)
			}
			all = append(all, e)
		}
		run.Stages["subtitle"] = "completed"
	} else if s.ASR != nil {
		run.Stages["subtitle"] = "running"
		_, _ = s.Store.PutAnalysisRun(run)
		cues, transcribeErr := s.ASR.Transcribe(ctx, asset)
		if transcribeErr != nil {
			return s.fail(run, "subtitle", transcribeErr)
		}
		for _, e := range asr.ToEvidence(req.ProjectID, asset, cues, req.Provider, req.AnalyzerVersion) {
			if _, err = s.Store.PutEvidence(e); err != nil {
				return s.fail(run, "subtitle", err)
			}
			all = append(all, e)
		}
		run.Stages["subtitle"] = "completed"
	} else {
		run.Stages["subtitle"] = "model_unavailable"
	}
	if req.Visual {
		run.Stages["visual"] = "running"
		_, _ = s.Store.PutAnalysisRun(run)
		frameDir := filepath.Join(s.Store.Dir, "analysis", asset.ID, key, "frames")
		frames, sampleErr := (visual.Sampler{}).Sample(ctx, s.Tools, asset, frameDir)
		if sampleErr != nil {
			return s.fail(run, "visual", sampleErr)
		}
		if s.Vision != nil {
			summaries, describeErr := s.Vision.Describe(ctx, frames)
			if describeErr != nil {
				return s.fail(run, "visual", describeErr)
			}
			for i := range frames {
				frames[i].VisualSummary = summaries[frames[i].ID]
			}
		}
		for _, e := range frames {
			if _, err = s.Store.PutEvidence(e); err != nil {
				return s.fail(run, "visual", err)
			}
			all = append(all, e)
		}
		run.Stages["visual"] = "completed"
	}
	if len(all) == 0 {
		if existing, existingErr := s.Store.Evidence(req.ProjectID, []string{req.AssetID}); existingErr == nil && len(existing) > 0 {
			all = existing
			run.Stages["indexed"] = "completed"
		}
	}
	if len(all) == 0 {
		return s.fail(run, "analysis", errors.New("model provider unavailable: provide subtitle_path or enable a visual sampler"))
	}
	for _, e := range all {
		run.EvidenceIDs = append(run.EvidenceIDs, e.ID)
	}
	run.Status = "completed"
	if _, err = s.Store.PutAnalysisRun(run); err != nil {
		return Result{}, err
	}
	return Result{Run: run, Evidence: all}, nil
}

func (s *Service) fail(run domain.AnalysisRun, stage string, err error) (Result, error) {
	run.Status = "failed"
	run.Error = err.Error()
	if run.Stages == nil {
		run.Stages = map[string]string{}
	}
	run.Stages[stage] = "failed"
	_, _ = s.Store.PutAnalysisRun(run)
	return Result{Run: run}, err
}

func (s *Service) resultFromRun(run domain.AnalysisRun) (Result, error) {
	items, err := s.Store.Evidence(run.ProjectID, []string{run.AssetID})
	if err != nil {
		return Result{}, err
	}
	allowed := map[string]bool{}
	for _, id := range run.EvidenceIDs {
		allowed[id] = true
	}
	filtered := items[:0]
	for _, e := range items {
		if allowed[e.ID] {
			filtered = append(filtered, e)
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].StartUS < filtered[j].StartUS })
	return Result{Run: run, Evidence: filtered}, nil
}

func SubtitlePath(path string) error {
	if path == "" {
		return errors.New("subtitle_path is required")
	}
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return fmt.Errorf("subtitle path is not a regular file")
	}
	return nil
}
