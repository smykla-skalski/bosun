package bot

import (
	"context"
	"sync"
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

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := executor.Close(ctx); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if executor.Submit(func(context.Context) {}) {
		t.Fatal("Submit() accepted work after Close()")
	}
}

func TestExecutorCloseRunsDropCallbacksConcurrently(t *testing.T) {
	const queuedJobs = 4
	executor, err := newExecutor(1, queuedJobs)
	if err != nil {
		t.Fatalf("newExecutor() error = %v", err)
	}
	workerStarted := make(chan struct{})
	if !executor.Submit(func(ctx context.Context) {
		close(workerStarted)
		<-ctx.Done()
	}) {
		t.Fatal("Submit() rejected the active job")
	}
	<-workerStarted

	dropsStarted := make(chan struct{}, queuedJobs)
	releaseDrops := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseDrops) }) })
	for range queuedJobs {
		if !executor.SubmitWithDrop(func(context.Context) {}, func() {
			dropsStarted <- struct{}{}
			<-releaseDrops
		}) {
			t.Fatal("SubmitWithDrop() rejected a queued job")
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	closed := make(chan error, 1)
	go func() { closed <- executor.Close(ctx) }()
	for range queuedJobs {
		select {
		case <-dropsStarted:
		case <-time.After(time.Second):
			t.Fatal("drop callbacks ran in series")
		}
	}
	releaseOnce.Do(func() { close(releaseDrops) })
	if err := <-closed; err != nil {
		t.Fatalf("Close() error = %v", err)
	}
}
