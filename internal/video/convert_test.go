package video

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func mediaFixture(t *testing.T, size string, audio bool) string {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Fatal("ffmpeg required:", err)
	}
	p := filepath.Join(t.TempDir(), "input.mp4")
	args := []string{"-v", "error", "-f", "lavfi", "-i", "testsrc2=size=" + size + ":rate=15:duration=1"}
	if audio {
		args = append(args, "-f", "lavfi", "-i", "sine=frequency=440:duration=1")
	}
	args = append(args, "-c:v", "libx264", "-pix_fmt", "yuv420p")
	if audio {
		args = append(args, "-c:a", "aac", "-shortest")
	}
	args = append(args, p)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	return p
}

func TestConvertShapesAndAudio(t *testing.T) {
	for _, tc := range []struct {
		size  string
		audio bool
	}{{"180x320", true}, {"320x180", true}, {"192x192", false}} {
		t.Run(tc.size, func(t *testing.T) {
			src := mediaFixture(t, tc.size, tc.audio)
			info, err := Probe(context.Background(), src)
			if err != nil || info.HasAudio != tc.audio {
				t.Fatalf("probe: %+v %v", info, err)
			}
			dst := filepath.Join(t.TempDir(), "circle.mp4")
			if err := ConvertToCircle(context.Background(), src, dst, CircleOptions{DurationSeconds: 0.6}); err != nil {
				t.Fatal(err)
			}
			got, err := Probe(context.Background(), dst)
			if err != nil || got.Width != 384 || got.Height != 384 || got.DurationSeconds > 0.67 || got.HasAudio != tc.audio {
				t.Fatalf("output: %+v %v", got, err)
			}
			if got.VideoCodec != "h264" || got.PixelFormat != "yuv420p" || got.SampleAspectRatio != "1:1" {
				t.Fatalf("incompatible: %+v", got)
			}
			if tc.audio && got.AudioCodec != "aac" {
				t.Fatal(got.AudioCodec)
			}
		})
	}
}

func TestProbeRejectsInvalidAndHDR(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.mp4")
	os.WriteFile(p, []byte("not a video"), 0600)
	if _, err := Probe(context.Background(), p); err == nil {
		t.Fatal("accepted corrupt video")
	}
	for _, duration := range []float64{0, -1, 601, math.NaN(), math.Inf(1)} {
		if err := ValidateInput(MediaInfo{Width: 192, Height: 192, DurationSeconds: duration}, 600); err == nil {
			t.Fatalf("accepted duration %v", duration)
		}
	}
	if err := ValidateInput(MediaInfo{Width: 192, Height: 192, DurationSeconds: 1, IsHDR: true}, 600); err == nil {
		t.Fatal("accepted HDR")
	}
}

func TestHEVCMOVAndLongInput(t *testing.T) {
	src := mediaFixture(t, "180x320", true)
	mov := filepath.Join(t.TempDir(), "iphone.mov")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-i", src, "-c:v", "libx265", "-x265-params", "pools=1:frame-threads=1", "-tag:v", "hvc1", "-c:a", "aac", mov).CombinedOutput(); err != nil {
		t.Fatalf("HEVC fixture: %v %s", err, out)
	}
	dst := filepath.Join(t.TempDir(), "circle.mp4")
	if err := ConvertToCircle(context.Background(), mov, dst, CircleOptions{DurationSeconds: 1}); err != nil {
		t.Fatal(err)
	}
	long := filepath.Join(t.TempDir(), "long.mp4")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=size=32x32:duration=61:rate=5", "-c:v", "libx264", "-pix_fmt", "yuv420p", long).CombinedOutput(); err != nil {
		t.Fatalf("long fixture: %v %s", err, out)
	}
	if err := ConvertToCircle(context.Background(), long, dst, CircleOptions{DurationSeconds: 60}); err != nil {
		t.Fatal(err)
	}
	info, err := Probe(context.Background(), dst)
	if err != nil || info.DurationSeconds > 60.01 || info.DurationSeconds < 59.9 {
		t.Fatalf("trim: %+v %v", info, err)
	}
}

func TestConvertCancelled(t *testing.T) {
	src := mediaFixture(t, "192x192", false)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	dst := filepath.Join(t.TempDir(), "cancelled.mp4")
	started := time.Now()
	if err := ConvertToCircle(ctx, src, dst, CircleOptions{DurationSeconds: 1}); err == nil {
		t.Fatal("cancel ignored")
	}
	if time.Since(started) > time.Second {
		t.Fatal("cancellation too slow")
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Fatal("partial output remains")
	}
}

func TestConvertRotationAndCentre(t *testing.T) {
	src := filepath.Join(t.TempDir(), "stripes.mp4")
	// A red centre with blue/green outer edges: central cropping must preserve red.
	filter := "color=blue:size=320x180:duration=1:rate=10,drawbox=x=70:y=0:w=180:h=180:color=red:t=fill,drawbox=x=250:y=0:w=70:h=180:color=green:t=fill"
	if out, err := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", filter, "-c:v", "libx264", "-pix_fmt", "yuv420p", src).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	rotated := filepath.Join(t.TempDir(), "rotated.mov")
	if out, err := exec.Command("ffmpeg", "-v", "error", "-i", src, "-c", "copy", "-metadata:s:v:0", "rotate=90", rotated).CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	dst := filepath.Join(t.TempDir(), "circle.mp4")
	if err := ConvertToCircle(context.Background(), rotated, dst, CircleOptions{DurationSeconds: 1}); err != nil {
		t.Fatal(err)
	}
	pixels, err := exec.Command("ffmpeg", "-v", "error", "-i", dst, "-frames:v", "1", "-vf", "scale=1:1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
	if err != nil || len(pixels) != 3 || pixels[0] < 180 || pixels[1] > 50 || pixels[2] > 50 {
		t.Fatalf("centre not red: %v %v", pixels, err)
	}
}
