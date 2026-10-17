package openrouter

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

func parseRetryAfter(value string, now time.Time) (time.Duration, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	if seconds, err := strconv.ParseInt(value, 10, 64); err == nil && seconds > 0 {
		if seconds > int64(maximumRetryDelay/time.Second) {
			return maximumRetryDelay, true
		}
		return time.Duration(seconds) * time.Second, true
	}
	date, err := http.ParseTime(value)
	if err != nil || !date.After(now) {
		return 0, false
	}
	return date.Sub(now), true
}

func IsAllowedModel(model string) bool {
	switch model {
	case ModelJev:
		return true
	default:
		return false
	}
}
