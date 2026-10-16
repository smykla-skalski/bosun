package main

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/events"
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

func TestGatewayReadyHandlersLogSafeTransitionFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		transition string
		log        func(*slog.Logger)
	}{
		{
			name:       "identified",
			transition: "identified",
			log: func(logger *slog.Logger) {
				gatewayReadyHandler(logger, 42)(&events.Ready{GenericEvent: events.NewGenericEvent(nil, 0, 3)})
			},
		},
		{
			name:       "resumed",
			transition: "resumed",
			log: func(logger *slog.Logger) {
				gatewayResumedHandler(logger, 42)(&events.Resumed{GenericEvent: events.NewGenericEvent(nil, 0, 3)})
			},
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var output bytes.Buffer
			test.log(slog.New(slog.NewJSONHandler(&output, nil)))
			logOutput := output.String()
			for _, field := range []string{`"gateway_state":"ready"`, `"transition":"` + test.transition + `"`, `"guild_id":"42"`, `"shard_id":3`} {
				if !strings.Contains(logOutput, field) {
					t.Fatalf("gateway log = %q, want %s", logOutput, field)
				}
			}
		})
	}
}
