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
	jobs   chan executorJob
	done   chan struct{}

	mu     sync.RWMutex
	closed bool
}

type executorJob struct {
	run  func(context.Context)
	drop func()
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
		jobs:   make(chan executorJob, capacity),
		done:   make(chan struct{}),
	}
	var workersDone sync.WaitGroup
	var dropsDone sync.WaitGroup
	workersDone.Add(workers)
	for range workers {
		go executor.work(&workersDone, &dropsDone)
	}
	go func() {
		workersDone.Wait()
		dropsDone.Wait()
		close(executor.done)
	}()
	return executor, nil
}

func (e *executor) Submit(job func(context.Context)) bool {
	return e.submit(executorJob{run: job})
}

func (e *executor) SubmitWithDrop(job func(context.Context), drop func()) bool {
	return e.submit(executorJob{run: job, drop: drop})
}

func (e *executor) submit(job executorJob) bool {
	if job.run == nil {
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

func (e *executor) work(workersDone *sync.WaitGroup, dropsDone *sync.WaitGroup) {
	defer workersDone.Done()
	for job := range e.jobs {
		if e.ctx.Err() != nil {
			if job.drop != nil {
				dropsDone.Add(1)
				go func() {
					defer dropsDone.Done()
					job.drop()
				}()
			}
			continue
		}
		job.run(e.ctx)
	}
}
