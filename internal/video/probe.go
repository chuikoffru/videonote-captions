package video

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os/exec"
	"strconv"
)

var ErrHDR = errors.New("HDR video requires SDR export")

type MediaInfo struct {
	DurationSeconds                                        float64
	Width, Height                                          int
	HasAudio, IsHDR                                        bool
	VideoCodec, AudioCodec, PixelFormat, SampleAspectRatio string
	VideoStreamIndex                                       int
}

// Restrict uploaded files to media containers, excluding playlists and network protocols.
const inputFormats = "mov,mp4,m4a,3gp,3g2,mj2,matroska,webm,avi,mpeg,mpegts"

func Probe(ctx context.Context, inputPath string) (MediaInfo, error) {
	var result MediaInfo
	cmd := exec.CommandContext(ctx, "ffprobe", "-v", "error", "-protocol_whitelist", "file,pipe", "-format_whitelist", inputFormats, "-show_streams", "-show_format", "-of", "json", inputPath)
	data, err := cmd.Output()
	if err != nil {
		return result, fmt.Errorf("cannot inspect video: %w", err)
	}
	var raw struct {
		Streams []struct {
			Index       int    `json:"index"`
			CodecType   string `json:"codec_type"`
			CodecName   string `json:"codec_name"`
			Width       int    `json:"width"`
			Height      int    `json:"height"`
			Duration    string `json:"duration"`
			PixFmt      string `json:"pix_fmt"`
			SAR         string `json:"sample_aspect_ratio"`
			Transfer    string `json:"color_transfer"`
			Disposition struct {
				AttachedPic int `json:"attached_pic"`
			} `json:"disposition"`
		} `json:"streams"`
		Format struct {
			Duration string `json:"duration"`
		} `json:"format"`
	}
	if err = json.Unmarshal(data, &raw); err != nil {
		return result, errors.New("invalid video metadata")
	}
	found := false
	for _, s := range raw.Streams {
		if s.CodecType == "audio" && !result.HasAudio {
			result.HasAudio = true
			result.AudioCodec = s.CodecName
		}
		if s.CodecType != "video" || s.Disposition.AttachedPic != 0 || found {
			continue
		}
		found = true
		result.Width = s.Width
		result.Height = s.Height
		result.VideoStreamIndex = s.Index
		result.VideoCodec = s.CodecName
		result.PixelFormat = s.PixFmt
		result.SampleAspectRatio = s.SAR
		result.IsHDR = s.Transfer == "smpte2084" || s.Transfer == "arib-std-b67"
		result.DurationSeconds, _ = strconv.ParseFloat(s.Duration, 64)
	}
	if !found {
		return result, errors.New("no video stream")
	}
	if result.DurationSeconds <= 0 || math.IsNaN(result.DurationSeconds) || math.IsInf(result.DurationSeconds, 0) {
		result.DurationSeconds, _ = strconv.ParseFloat(raw.Format.Duration, 64)
	}
	if result.Width <= 0 || result.Height <= 0 || result.DurationSeconds <= 0 || math.IsNaN(result.DurationSeconds) || math.IsInf(result.DurationSeconds, 0) {
		return result, errors.New("invalid video dimensions or duration")
	}
	return result, nil
}

func ValidateInput(info MediaInfo, maxSeconds int) error {
	if info.IsHDR {
		return ErrHDR
	}
	if info.Width <= 0 || info.Height <= 0 || info.DurationSeconds <= 0 || math.IsNaN(info.DurationSeconds) || math.IsInf(info.DurationSeconds, 0) || info.DurationSeconds > float64(maxSeconds) {
		return errors.New("video duration or dimensions outside limits")
	}
	return nil
}
