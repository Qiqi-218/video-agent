package provider

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/zylar06/video-agent/internal/analysis/asr"
	"github.com/zylar06/video-agent/internal/domain"
)

type Config struct {
	BaseURL, Model, APIKey string
	HTTPClient             *http.Client
}

func ConfigFromEnv(prefix string) Config {
	return Config{BaseURL: os.Getenv(prefix + "_BASE_URL"), Model: os.Getenv(prefix + "_MODEL"), APIKey: os.Getenv(prefix + "_API_KEY"), HTTPClient: http.DefaultClient}
}

type OpenAITranscriber struct{ Config Config }

func (p OpenAITranscriber) Transcribe(ctx context.Context, asset domain.MediaAsset) ([]asr.Cue, error) {
	if p.Config.BaseURL == "" || p.Config.Model == "" || p.Config.APIKey == "" {
		return nil, errors.New("model provider unavailable: ASR provider is not configured")
	}
	f, err := os.Open(asset.Path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	if err := form.WriteField("model", p.Config.Model); err != nil {
		return nil, err
	}
	if err := form.WriteField("response_format", "verbose_json"); err != nil {
		return nil, err
	}
	if err := form.WriteField("timestamp_granularities[]", "segment"); err != nil {
		return nil, err
	}
	part, err := form.CreateFormFile("file", filepath.Base(asset.Path))
	if err != nil {
		return nil, err
	}
	if _, err = io.Copy(part, f); err != nil {
		return nil, err
	}
	if err = form.Close(); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(p.Config.BaseURL, "audio/transcriptions"), &body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.Config.APIKey)
	req.Header.Set("Content-Type", form.FormDataContentType())
	resp, err := client(p.Config).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if resp.StatusCode/100 != 2 {
		return nil, fmt.Errorf("ASR provider returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	var decoded struct {
		Text     string `json:"text"`
		Segments []struct {
			Start float64 `json:"start"`
			End   float64 `json:"end"`
			Text  string  `json:"text"`
		} `json:"segments"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		return nil, fmt.Errorf("invalid ASR response: %w", err)
	}
	if len(decoded.Segments) > 0 {
		out := make([]asr.Cue, 0, len(decoded.Segments))
		for i, seg := range decoded.Segments {
			if strings.TrimSpace(seg.Text) != "" && seg.End > seg.Start {
				out = append(out, asr.Cue{Index: i + 1, StartUS: int64(seg.Start*1e6 + 0.5), EndUS: int64(seg.End*1e6 + 0.5), Text: strings.TrimSpace(seg.Text)})
			}
		}
		return out, nil
	}
	if strings.TrimSpace(decoded.Text) == "" {
		return nil, errors.New("ASR provider returned empty transcript")
	}
	return []asr.Cue{{Index: 1, StartUS: 0, EndUS: asset.DurationUS, Text: strings.TrimSpace(decoded.Text)}}, nil
}

type OpenAIVision struct{ Config Config }

func (p OpenAIVision) Describe(ctx context.Context, evidence []domain.Evidence) (map[string]string, error) {
	if p.Config.BaseURL == "" || p.Config.Model == "" || p.Config.APIKey == "" {
		return nil, errors.New("model provider unavailable: vision provider is not configured")
	}
	out := map[string]string{}
	for _, e := range evidence {
		if len(e.FrameRefs) == 0 {
			continue
		}
		data, err := os.ReadFile(e.FrameRefs[0])
		if err != nil {
			return nil, err
		}
		payload := map[string]any{"model": p.Config.Model, "temperature": 0, "messages": []any{map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "用一句简洁中文描述这帧画面中可核对的主体、动作和场景；不要猜测画外信息。"}, map[string]any{"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data)}}}}}}
		b, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint(p.Config.BaseURL, "chat/completions"), bytes.NewReader(b))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+p.Config.APIKey)
		req.Header.Set("Content-Type", "application/json")
		resp, err := client(p.Config).Do(req)
		if err != nil {
			return nil, err
		}
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
		resp.Body.Close()
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("vision provider returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		}
		var decoded struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &decoded); err != nil || len(decoded.Choices) == 0 || strings.TrimSpace(decoded.Choices[0].Message.Content) == "" {
			return nil, errors.New("vision provider returned empty description")
		}
		out[e.ID] = strings.TrimSpace(decoded.Choices[0].Message.Content)
	}
	return out, nil
}

func endpoint(base, suffix string) string {
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1") {
		return base + "/" + suffix
	}
	return base + "/v1/" + suffix
}
func client(c Config) *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}
