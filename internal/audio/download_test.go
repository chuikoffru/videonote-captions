package audio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDownloadLimitsAndCleanup(t *testing.T) {
	for _, mode := range []string{"ok", "declared", "stream", "status", "cancel"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if mode == "status" {
					w.WriteHeader(503)
					return
				}
				if mode == "declared" {
					w.Header().Set("Content-Length", "100")
				}
				if mode == "stream" {
					w.(http.Flusher).Flush()
				}
				w.Write([]byte("123456789"))
			}))
			defer server.Close()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			p := filepath.Join(t.TempDir(), "video.mp4")
			limit := int64(8)
			if mode == "ok" {
				limit = 9
			}
			err := downloadTo(ctx, server.Client(), server.URL, p, limit)
			if mode == "ok" {
				if err != nil {
					t.Fatal(err)
				}
				data, _ := os.ReadFile(p)
				if string(data) != "123456789" {
					t.Fatal(string(data))
				}
			} else {
				if err == nil {
					t.Fatal("expected download failure")
				}
				if _, err := os.Stat(p); !os.IsNotExist(err) {
					t.Fatal("partial file leaked")
				}
			}
		})
	}
}
