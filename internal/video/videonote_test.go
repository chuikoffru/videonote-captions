package video

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"videonote-captions/internal/transcription"
)

func TestOneWordTiming(t *testing.T) {
	path, err := createWordASS(&transcription.Result{Words: []transcription.Word{
		{Word: "Привет", Start: 0.2, End: 0.6},
		{Word: "мир", Start: 1.1, End: 1.5},
	}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Dialogue:") {
			lines = append(lines, line)
		}
	}
	if len(lines) != 2 {
		t.Fatalf("want separate event per word, got %v", lines)
	}
	if !strings.Contains(lines[0], "0:00:00.20,0:00:00.60") || !strings.HasSuffix(lines[0], ",Привет") {
		t.Fatal(lines[0])
	}
	if !strings.Contains(lines[1], "0:00:01.10,0:00:01.50") || !strings.HasSuffix(lines[1], ",мир") {
		t.Fatal(lines[1])
	}
}

func TestWordsNeverOverlap(t *testing.T) {
	path, err := createWordASS(&transcription.Result{Words: []transcription.Word{
		{Word: "one", Start: 0, End: 1}, {Word: "two", Start: 0.5, End: 1.5},
		{Word: "", Start: 2, End: 3}, {Word: "invalid", Start: 4, End: 3},
	}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Count(string(data), "Dialogue:") != 2 || !strings.Contains(string(data), "0:00:00.00,0:00:00.50") {
		t.Fatal(string(data))
	}
}

func TestRejectsMissingWordTimestamps(t *testing.T) {
	path, err := createWordASS(&transcription.Result{Text: "no timings"}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	if strings.Contains(string(data), "Dialogue:") {
		t.Fatal("invented word timing")
	}
}

func TestSubtitleTextCannotInjectASS(t *testing.T) {
	path, err := createWordASS(&transcription.Result{Words: []transcription.Word{{Word: "{\\pos(0,0)}hello", Start: 0, End: 1}}}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "Dialogue:") && strings.Contains(line, "{\\pos") {
			t.Fatal("ASS injection")
		}
	}
}

// Real FFmpeg check: the pause must match the input frame; a spoken interval must differ.
func TestRenderedWordDisappearsInPause(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg unavailable")
	}
	filters, _ := exec.Command("ffmpeg", "-hide_banner", "-filters").CombinedOutput()
	if !strings.Contains(string(filters), " subtitles ") {
		t.Skip("ffmpeg requires libass subtitles filter")
	}
	dir := t.TempDir()
	input := filepath.Join(dir, "input.mp4")
	cmd := exec.Command("ffmpeg", "-v", "error", "-f", "lavfi", "-i", "color=c=0x203040:s=384x384:d=2:r=25", "-f", "lavfi", "-i", "sine=frequency=440:duration=2", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-c:a", "aac", "-shortest", input)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("fixture: %v %s", err, out)
	}
	output, err := AddSubtitlesToVideoNote(input, &transcription.Result{Words: []transcription.Word{{Word: "Привет", Start: 0.2, End: 0.6}, {Word: "мир", Start: 1.1, End: 1.5}}})
	if err != nil {
		t.Fatal(err)
	}
	frame := func(at string) []byte {
		data, e := exec.Command("ffmpeg", "-v", "error", "-ss", at, "-i", output, "-frames:v", "1", "-f", "rawvideo", "-pix_fmt", "rgb24", "-").Output()
		if e != nil {
			t.Fatal(e)
		}
		return data
	}
	blank, word, pause := frame("0.08"), frame("0.40"), frame("0.84")
	difference := func(a, b []byte) int {
		n := 0
		for i := range a {
			d := int(a[i]) - int(b[i])
			if d > 30 || d < -30 {
				n++
			}
		}
		return n
	}
	if difference(blank, word) < 100 {
		t.Fatal("word not visible")
	}
	if difference(blank, pause) > 100 {
		t.Fatal("subtitle remains in pause")
	}
	files, _ := filepath.Glob(filepath.Join(dir, "*.ass"))
	if len(files) != 0 {
		t.Fatal("temporary subtitles leaked")
	}
}
