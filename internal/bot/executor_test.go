package bot

import (
	"context"
	"testing"
	"time"
)

func TestExecutorRejectsWorkWhenQueueIsFull(t *testing.T) {
	t.Parallel()

	executor, err := newExecutor(1, 1)
	if err != nil {
		t.Fatalf("newExecutor() error = %v", err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	if !executor.Submit(func(context.Context) {
		close(started)
		<-release
	}) {
		t.Fatal("Submit() rejected the first job")
	}
	<-started
	if !executor.Submit(func(context.Context) {}) {
		t.Fatal("Submit() rejected the queued job")
	}
	if executor.Submit(func(context.Context) {}) {
		t.Fatal("Submit() accepted work beyond queue capacity")
	}
	close(release)
	if err := executor.Close(context.Background()); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}

func TestExecutorCloseCancelsRunningWork(t *testing.T) {
	t.Parallel()

	executor, err := newExecutor(1, 1)
	if err != nil {
		t.Fatalf("newExecutor() error = %v", err)
	}
	started := make(chan struct{})
	if !executor.Submit(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
	}) {
		t.Fatal("Submit() rejected the job")
	}
	<-started

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := executor.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if executor.Submit(func(context.Context) {}) {
		t.Fatal("Submit() accepted work after Close()")
	}
}
