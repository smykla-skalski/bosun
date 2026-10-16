package bot

import (
	"context"
	"errors"
	"sync"
)

var errExecutorShutdown = errors.New("executor shutdown timed out")

type executor struct {
	ctx    context.Context
	cancel context.CancelFunc
	jobs   chan func(context.Context)
	done   chan struct{}

	mu     sync.RWMutex
	closed bool
}

func newExecutor(workers int, capacity int) (*executor, error) {
	if workers < 1 {
		return nil, errors.New("message executor needs a worker")
	}
	if capacity < 1 {
		return nil, errors.New("message executor needs queue capacity")
	}

	ctx, cancel := context.WithCancel(context.Background())
	executor := &executor{
		ctx:    ctx,
		cancel: cancel,
		jobs:   make(chan func(context.Context), capacity),
		done:   make(chan struct{}),
	}
	var workersDone sync.WaitGroup
	workersDone.Add(workers)
	for range workers {
		go executor.work(&workersDone)
	}
	go func() {
		workersDone.Wait()
		close(executor.done)
	}()
	return executor, nil
}

func (e *executor) Submit(job func(context.Context)) bool {
	if job == nil {
		return false
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	if e.closed {
		return false
	}
	select {
	case e.jobs <- job:
		return true
	default:
		return false
	}
}

func (e *executor) Context() context.Context {
	return e.ctx
}

func (e *executor) Close(ctx context.Context) error {
	e.mu.Lock()
	if !e.closed {
		e.closed = true
		e.cancel()
		close(e.jobs)
	}
	e.mu.Unlock()

	select {
	case <-e.done:
		return nil
	case <-ctx.Done():
		return errExecutorShutdown
	}
}

func (e *executor) work(done *sync.WaitGroup) {
	defer done.Done()
	for {
		select {
		case <-e.ctx.Done():
			return
		case job, ok := <-e.jobs:
			if !ok || e.ctx.Err() != nil {
				return
			}
			job(e.ctx)
		}
	}
}
