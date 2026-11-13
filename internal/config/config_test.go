package config

import "testing"

func TestVideoConversionDefaultsAndLimits(t *testing.T) {
	for _, key := range []string{"ENABLE_VIDEO_TO_CIRCLE", "PLAIN_CIRCLES_PER_DAY", "MAX_INPUT_VIDEO_BYTES", "MAX_INPUT_VIDEO_SECONDS", "VIDEO_PROCESS_TIMEOUT_SECONDS"} {
		t.Setenv(key, "")
	}
	cfg := Load()
	if cfg.EnableVideoToCircle || cfg.PlainCirclesPerDay != 5 || cfg.MaxInputVideoBytes != 20971520 || cfg.MaxInputVideoSeconds != 600 || cfg.VideoProcessTimeoutSeconds != 180 {
		t.Fatalf("defaults %+v", cfg)
	}
	t.Setenv("ENABLE_VIDEO_TO_CIRCLE", "true")
	t.Setenv("PLAIN_CIRCLES_PER_DAY", "7")
	t.Setenv("MAX_INPUT_VIDEO_BYTES", "999999999")
	t.Setenv("MAX_INPUT_VIDEO_SECONDS", "-2")
	t.Setenv("VIDEO_PROCESS_TIMEOUT_SECONDS", "bad")
	cfg = Load()
	if !cfg.EnableVideoToCircle || cfg.PlainCirclesPerDay != 7 || cfg.MaxInputVideoBytes != 20971520 || cfg.MaxInputVideoSeconds != 600 || cfg.VideoProcessTimeoutSeconds != 180 {
		t.Fatalf("limits %+v", cfg)
	}
}
