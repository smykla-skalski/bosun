package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestWaitForExitReturnsGatewayTermination(t *testing.T) {
	t.Parallel()

	healthErrors := make(chan error)
	terminated := make(chan struct{}, 1)
	gatewayCloseHandler(terminated)(nil, errors.New("terminal close"), false)

	err := waitForExit(context.Background(), healthErrors, terminated)
	if !errors.Is(err, errGatewayTerminated) {
		t.Fatalf("waitForExit() error = %v, want gateway termination", err)
	}
}

func TestWaitForExitReturnsAfterContextCancellation(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := waitForExit(ctx, make(chan error), make(chan struct{})); err != nil {
		t.Fatalf("waitForExit() error = %v, want nil", err)
	}
}

func TestGatewayCloseHandlerNeverBlocks(t *testing.T) {
	t.Parallel()

	terminated := make(chan struct{}, 1)
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
