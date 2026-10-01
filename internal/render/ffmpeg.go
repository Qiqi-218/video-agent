package render

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/zylar06/video-agent/internal/domain"
	"github.com/zylar06/video-agent/internal/media"
)

// Arguments normalizes each segment before concatenation. There is no stream copy:
// arbitrary source in/out points therefore work between keyframes as well.
func Arguments(p Plan, target string) []string {
	args := []string{"-hide_banner", "-v", "error", "-nostdin", "-y", "-xerror", "-filter_complex_threads", "1"}
	for _, i := range p.Inputs {
		args = append(args, "-protocol_whitelist", "file,pipe", "-err_detect", "explode", "-ss", media.Seconds(i.SourceInUS), "-t", media.Seconds(i.SourceOutUS-i.SourceInUS), "-i", i.Path)
	}
	filters := []string{}
	labels := []string{}
	rate := fmt.Sprintf("%d/%d", p.FPSNum, p.FPSDen)
	for n, i := range p.Inputs {
		// Pad the tail before trimming to the exact requested frame count. This
		// handles frame-rate conversion at the boundary of a validated source range.
		filters = append(filters, fmt.Sprintf("[%d:v:0]setpts=PTS-STARTPTS,scale=%d:%d:force_original_aspect_ratio=decrease:force_divisible_by=2,pad=%d:%d:(ow-iw)/2:(oh-ih)/2,setsar=1,fps=%s,tpad=stop_mode=clone:stop_duration=1,trim=end_frame=%d,setpts=N*%d/(%d*TB),format=yuv420p[v%d]", n, p.Width, p.Height, p.Width, p.Height, rate, i.DurationFrames, p.FPSDen, p.FPSNum, n))
		samples := int64(math.Round(float64(i.DurationFrames) * 48000 * float64(p.FPSDen) / float64(p.FPSNum)))
		if i.HasAudio {
			filters = append(filters, fmt.Sprintf("[%d:a:0]asetpts=PTS-STARTPTS,aresample=48000:async=1:first_pts=0,aformat=sample_fmts=fltp:channel_layouts=stereo,apad,atrim=end_sample=%d,asetpts=N/SR/TB[a%d]", n, samples, n))
		} else {
			filters = append(filters, fmt.Sprintf("anullsrc=r=48000:cl=stereo,atrim=end_sample=%d,asetpts=N/SR/TB[a%d]", samples, n))
		}
		labels = append(labels, fmt.Sprintf("[v%d][a%d]", n, n))
	}
	filters = append(filters, fmt.Sprintf("%sconcat=n=%d:v=1:a=1[v][a]", strings.Join(labels, ""), len(p.Inputs)))
	return append(args, "-filter_complex", strings.Join(filters, ";"), "-map", "[v]", "-map", "[a]", "-map_metadata", "-1", "-map_chapters", "-1", "-c:v", "libx264", "-preset", "veryfast", "-crf", "20", "-threads", "2", "-pix_fmt", "yuv420p", "-r", rate, "-fps_mode", "cfr", "-c:a", "aac", "-b:a", "128k", "-ar", "48000", "-ac", "2", "-t", media.Seconds(p.DurationUS()), "-movflags", "+faststart", "-f", "mp4", target)
}

func Execute(ctx context.Context, tools media.Tools, p Plan, target string) (domain.Validation, error) {
	if err := p.Validate(); err != nil {
		return domain.Validation{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Hour)
	defer cancel()
	if !strings.EqualFold(filepath.Ext(target), ".mp4") {
		return domain.Validation{}, errors.New("output must end in .mp4")
	}
	target, err := filepath.Abs(target)
	if err != nil {
		return domain.Validation{}, err
	}
	if _, err = os.Lstat(target); err == nil {
		return domain.Validation{}, errors.New("output already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.Validation{}, err
	}
	if err = os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return domain.Validation{}, err
	}
	seen := map[string]bool{}
	for _, i := range p.Inputs {
		if seen[i.AssetID] {
			continue
		}
		seen[i.AssetID] = true
		hash, e := media.Hash(ctx, i.Path)
		if e != nil {
			return domain.Validation{}, e
		}
		if hash != i.ContentHash {
			return domain.Validation{}, fmt.Errorf("asset %s changed after import", i.AssetID)
		}
		info, e := tools.Probe(ctx, i.Path)
		if e != nil {
			return domain.Validation{}, e
		}
		if info.HasAudio != i.HasAudio {
			return domain.Validation{}, errors.New("asset audio metadata mismatch")
		}
		for _, clip := range p.Inputs {
			if clip.AssetID == i.AssetID && clip.SourceOutUS > info.DurationUS {
				return domain.Validation{}, errors.New("source range exceeds actual video duration")
			}
		}
	}
	tmp, err := os.CreateTemp(filepath.Dir(target), ".video-agent-*.mp4")
	if err != nil {
		return domain.Validation{}, err
	}
	path := tmp.Name()
	tmp.Close()
	defer os.Remove(path)
	if _, err = media.Run(ctx, tools.FFmpeg, Arguments(p, path)...); err != nil {
		return domain.Validation{}, err
	}
	validation, err := Verify(ctx, tools, p, path)
	if err != nil {
		return validation, err
	}
	if err = ctx.Err(); err != nil {
		return validation, err
	}
	// Hard-link publication is atomic and fails if a concurrent export won the name.
	if err = os.Link(path, target); err != nil {
		return validation, err
	}
	return validation, nil
}

func Verify(ctx context.Context, tools media.Tools, p Plan, path string) (domain.Validation, error) {
	info, err := tools.Probe(ctx, path)
	if err != nil {
		return domain.Validation{}, err
	}
	v := domain.Validation{Width: info.Width, Height: info.Height, Frames: info.Frames, DurationUS: info.DurationUS}
	wantRate := float64(p.FPSNum) / float64(p.FPSDen)
	if info.Width != p.Width || info.Height != p.Height || info.Frames != p.Frames() || info.VideoCodec != "h264" || info.AudioCodec != "aac" || info.AudioChannels != 2 || math.Abs(media.Rate(info.FPS)-wantRate) > 0.001 || math.Abs(float64(info.DurationUS-p.DurationUS())) > 1e6/wantRate+1000 {
		return v, fmt.Errorf("output metadata mismatch: got %+v; want %d frames at %dx%d", info, p.Frames(), p.Width, p.Height)
	}
	v.ProbePassed = true
	if _, err = media.Run(ctx, tools.FFmpeg, "-hide_banner", "-v", "error", "-nostdin", "-xerror", "-err_detect", "explode", "-protocol_whitelist", "file,pipe", "-i", path, "-map", "0:v:0", "-map", "0:a:0", "-f", "null", "-"); err != nil {
		return v, fmt.Errorf("full decode: %w", err)
	}
	v.DecodePassed = true
	v.SHA256, err = media.Hash(ctx, path)
	return v, err
}
