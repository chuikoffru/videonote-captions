package video

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"os/exec"
	"strconv"
)

type CircleOptions struct{ DurationSeconds float64 }

const MaxCircleBytes int64 = 10 * 1024 * 1024

func ConvertToCircle(ctx context.Context, inputPath, outputPath string, opts CircleOptions) (err error) {
	if opts.DurationSeconds <= 0 || opts.DurationSeconds > 60 || math.IsNaN(opts.DurationSeconds) || math.IsInf(opts.DurationSeconds, 0) {
		return errors.New("invalid circle duration")
	}
	defer func() {
		if err != nil {
			os.Remove(outputPath)
		}
	}()
	info, err := Probe(ctx, inputPath)
	if err != nil {
		return err
	}
	if info.IsHDR {
		return ErrHDR
	}
	// FFmpeg applies display-matrix rotation before these filters. Normalize SAR before cropping.
	filter := "scale=iw*sar:ih,setsar=1,crop='min(iw,ih)':'min(iw,ih)',scale=384:384:flags=lanczos,setsar=1,fps=30"
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-nostdin", "-y", "-threads", "2", "-protocol_whitelist", "file,pipe", "-format_whitelist", inputFormats, "-i", inputPath,
		"-map", fmt.Sprintf("0:%d", info.VideoStreamIndex), "-map", "0:a:0?", "-sn", "-dn", "-t", strconv.FormatFloat(opts.DurationSeconds, 'f', 6, 64),
		"-vf", filter, "-c:v", "libx264", "-threads", "2", "-preset", "fast", "-crf", "23", "-maxrate", "900k", "-bufsize", "1800k", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-b:a", "96k", "-ac", "2", "-ar", "48000", "-map_metadata", "-1", "-metadata:s:v:0", "rotate=0", "-movflags", "+faststart", outputPath)
	if out, e := cmd.CombinedOutput(); e != nil {
		return fmt.Errorf("circle conversion failed: %w (%s)", e, out)
	}
	output, err := Probe(ctx, outputPath)
	if err != nil {
		return err
	}
	return ValidateCircleOutput(outputPath, output)
}

func ValidateCircleOutput(path string, info MediaInfo) error {
	stat, err := os.Stat(path)
	if err != nil {
		return err
	}
	if stat.Size() == 0 || stat.Size() > MaxCircleBytes {
		return errors.New("circle file outside size limit")
	}
	if info.Width != info.Height || info.Width <= 0 || info.DurationSeconds <= 0 || info.DurationSeconds > 60.05 || math.IsNaN(info.DurationSeconds) || math.IsInf(info.DurationSeconds, 0) {
		return errors.New("invalid circle geometry or duration")
	}
	if info.VideoCodec != "h264" || info.PixelFormat != "yuv420p" {
		return errors.New("incompatible circle encoding")
	}
	if info.HasAudio && info.AudioCodec != "aac" {
		return errors.New("incompatible circle audio")
	}
	return nil
}
