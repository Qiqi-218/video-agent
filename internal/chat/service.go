package chat

import (
	"context"
	"encoding/json"
	"regexp"
	"strconv"
	"strings"

	"github.com/zylar06/video-agent/internal/analysis"
	"github.com/zylar06/video-agent/internal/analysis/provider"
	"github.com/zylar06/video-agent/internal/catalog"
	"github.com/zylar06/video-agent/internal/store"
)

type Request struct {
	ProjectID    string `json:"project_id"`
	AssetID      string `json:"asset_id"`
	Message      string `json:"message"`
	SubtitlePath string `json:"subtitle_path,omitempty"`
	Visual       bool   `json:"visual,omitempty"`
}

type Intent struct {
	Goal       string `json:"goal"`
	Query      string `json:"query,omitempty"`
	DurationUS int64  `json:"duration_us,omitempty"`
	NeedsEdit  bool   `json:"needs_edit"`
}

type Result struct {
	Reply    string                 `json:"reply"`
	Intent   Intent                 `json:"intent"`
	Evidence []catalog.SearchResult `json:"evidence,omitempty"`
}

type Service struct {
	Store    *store.Store
	Text     provider.OpenAIText
	Analyzer *analysis.Service
}

func (s Service) Handle(ctx context.Context, req Request) (Result, error) {
	intent := classify(req.Message)
	if s.Analyzer != nil && (req.SubtitlePath != "" || req.Visual) {
		if _, err := s.Analyzer.Analyze(ctx, analysis.Request{ProjectID: req.ProjectID, AssetID: req.AssetID, SubtitlePath: req.SubtitlePath, Visual: req.Visual}); err != nil {
			return Result{}, err
		}
	}
	if s.Text.Config.BaseURL != "" && s.Text.Config.Model != "" && s.Text.Config.APIKey != "" {
		prompt := "将用户请求映射为 JSON，仅允许 goal=highlights,jokes,sports,trim_ends,preview；字段 query、duration_us、needs_edit。没有明确时长填 0。用户请求：" + req.Message
		if raw, err := s.Text.Complete(ctx, prompt); err == nil {
			var modelIntent Intent
			if json.Unmarshal([]byte(raw), &modelIntent) == nil && valid(modelIntent.Goal) {
				// Keep the local, searchable vocabulary stable; the model supplies
				// intent and duration, but must not invent an unindexed query.
				modelIntent.Query = intent.Query
				if modelIntent.DurationUS == 0 {
					modelIntent.DurationUS = intent.DurationUS
				}
				modelIntent.NeedsEdit = true
				intent = modelIntent
			}
		}
	}
	if intent.Query != "" {
		items, err := s.Store.SearchEvidence(req.ProjectID, intent.Query, []string{req.AssetID}, 12)
		if err == nil {
			return Result{Reply: reply(intent, len(items)), Intent: intent, Evidence: items}, nil
		}
	}
	return Result{Reply: reply(intent, 0), Intent: intent}, nil
}

var durationRE = regexp.MustCompile(`([0-9]+)\s*(分钟|分|秒)`)

func classify(message string) Intent {
	m := strings.ToLower(strings.TrimSpace(message))
	intent := Intent{Goal: "highlights", Query: "高能", NeedsEdit: true}
	switch {
	case strings.Contains(m, "笑"):
		intent = Intent{Goal: "jokes", Query: "笑", NeedsEdit: true}
	case strings.Contains(m, "进球") || strings.Contains(m, "庆祝"):
		intent = Intent{Goal: "sports", Query: "进球 庆祝", NeedsEdit: true}
	case strings.Contains(m, "开场") || strings.Contains(m, "片尾"):
		intent = Intent{Goal: "trim_ends", NeedsEdit: true}
	case strings.Contains(m, "预览"):
		intent = Intent{Goal: "preview", NeedsEdit: true}
	}
	if match := durationRE.FindStringSubmatch(m); len(match) == 3 {
		n, _ := strconv.ParseInt(match[1], 10, 64)
		if match[2] == "秒" {
			intent.DurationUS = n * 1_000_000
		} else {
			intent.DurationUS = n * 60 * 1_000_000
		}
	}
	return intent
}

func valid(goal string) bool {
	return goal == "highlights" || goal == "jokes" || goal == "sports" || goal == "trim_ends" || goal == "preview"
}
func reply(i Intent, count int) string {
	if i.Goal == "trim_ends" {
		return "我可以生成删除开场和片尾的剪辑方案；先确认时间线后再修改。"
	}
	if i.Goal == "preview" {
		return "我可以生成当前剪辑方案的预览；请先确认候选片段。"
	}
	if count == 0 {
		return "我暂时没有找到可核对的候选片段，可以换一个关键词或提供字幕。"
	}
	return "我找到 " + strconv.Itoa(count) + " 个有来源的候选片段。请确认后，我再生成剪辑方案。"
}
