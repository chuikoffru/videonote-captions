// Package attribution validates Telegram command tokens and start parameters.
package attribution

import (
	"regexp"
	"strings"
)

var commandPattern = regexp.MustCompile(`^/([a-z][a-z0-9_]*)(@[A-Za-z0-9_]+)?$`)
var sourcePattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)

// Command returns the exact command without its slash or optional bot suffix.
// The caller compares the returned name; /starter never matches /start.
func Command(text string) string {
	if text == "" || text[0] != '/' {
		return ""
	}
	fields := strings.Fields(text)
	if len(fields) == 0 {
		return ""
	}
	match := commandPattern.FindStringSubmatch(fields[0])
	if match == nil {
		return ""
	}
	return match[1]
}

// ParseStart accepts one Telegram-safe parameter; no parameter means direct.
func ParseStart(text string) (source, language string, valid bool) {
	if Command(text) != "start" {
		return "", "", false
	}
	fields := strings.Fields(text)
	if len(fields) == 1 {
		return "direct", "", true
	}
	if len(fields) != 2 || !sourcePattern.MatchString(fields[1]) {
		return "", "", false
	}
	source = fields[1]
	if strings.HasPrefix(source, "ru_") {
		language = "ru"
	}
	if strings.HasPrefix(source, "en_") {
		language = "en"
	}
	return source, language, true
}

// ValidSource protects persistence when RecordStart is called outside a handler.
func ValidSource(source string) bool { return sourcePattern.MatchString(source) }
