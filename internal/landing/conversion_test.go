package landing

import (
	"strings"
	"testing"
)

func TestConversionCopyIsFeatureGatedAndLocalized(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, lang := range []string{"ru", "en"} {
			c := testConfig()
			c.EnableVideoToCircle = enabled
			c.PlainCirclesPerDay = 5
			body := response(c, "/"+lang+"/").Body.String()
			needle := "video_to_circle"
			if strings.Contains(body, needle) != enabled {
				t.Fatalf("visibility enabled=%v %s", enabled, lang)
			}
			if enabled {
				if !strings.Contains(body, "5") || !strings.Contains(body, "20") || !strings.Contains(body, "60") {
					t.Fatal("missing limits")
				}
				for _, path := range []string{"terms/", "privacy/"} {
					legal := response(c, "/"+lang+"/"+path).Body.String()
					if strings.Contains(legal, "regular video file support are not included") || strings.Contains(legal, "обработка обычных видеофайлов не входят") {
						t.Fatal("obsolete terms")
					}
				}
			}
		}
	}
}
