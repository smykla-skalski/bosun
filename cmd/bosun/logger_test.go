package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestDependencyLoggerDropsAllDependencyData(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&output, &slog.HandlerOptions{Level: slog.LevelDebug}))
	logger := dependencyLogger(base).With("name", "dependency")
	logger.Debug("request", "body", "private message text")
	logger.Error("request failed", "body", "private reply text")

	got := output.String()
	if strings.Contains(got, "private message text") || strings.Contains(got, "private reply text") || strings.Contains(got, "request") {
		t.Fatal("dependency logger wrote dependency data")
	}
}
