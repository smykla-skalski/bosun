package main

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestWaitForExitReturnsGatewayTermination(t *testing.T) {
	t.Parallel()

	healthErrors := make(chan error)
	terminated := make(chan gatewayTermination, 1)
	gatewayCloseHandler(terminated)(nil, errors.New("private terminal close"), false)
	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))

	err := waitForExit(context.Background(), healthErrors, terminated, logger, 42)
	if !errors.Is(err, errGatewayTerminated) {
		t.Fatalf("waitForExit() error = %v, want gateway termination", err)
	}
	logOutput := output.String()
	for _, field := range []string{`"gateway_state":"disconnected"`, `"transition":"terminal_close"`, `"guild_id":"42"`, `"shard_id":0`, `"error_type":"*errors.errorString"`} {
		if !strings.Contains(logOutput, field) {
			t.Fatalf("terminal gateway log = %q, want %s", logOutput, field)
		}
	}
	if strings.Contains(logOutput, "private terminal close") {
		t.Fatalf("terminal gateway log contains the raw error: %s", logOutput)
	}
}

func TestWaitForExitReturnsAfterContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	logger := slog.New(slog.NewJSONHandler(&bytes.Buffer{}, nil))
	if err := waitForExit(ctx, make(chan error), make(chan gatewayTermination), logger, 42); err != nil {
		t.Fatalf("waitForExit() error = %v, want nil", err)
	}
}

func TestGatewayCloseHandlerNeverBlocks(t *testing.T) {
	t.Parallel()

	terminated := make(chan gatewayTermination, 1)
	handler := gatewayCloseHandler(terminated)
	handler(nil, errors.New("first close"), false)
	done := make(chan struct{})
	go func() {
		handler(nil, errors.New("second close"), false)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("gateway close handler blocked on a full channel")
	}
}
