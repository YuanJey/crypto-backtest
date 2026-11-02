package market

import (
	"fmt"
	"strings"
	"time"
)

func ParseUTCMS(value string) (int64, error) {
	text := strings.TrimSpace(value)
	if text == "" {
		return 0, fmt.Errorf("time value is empty")
	}
	if len(text) == len("2006-01-02") && !strings.Contains(text, "T") {
		text += "T00:00:00Z"
	}

	t, err := time.Parse(time.RFC3339, text)
	if err != nil {
		return 0, fmt.Errorf("parse UTC time %q: %w", value, err)
	}
	return t.UTC().UnixMilli(), nil
}

func ISOUTCMS(valueMS int64) string {
	return time.UnixMilli(valueMS).UTC().Format(time.RFC3339)
}
