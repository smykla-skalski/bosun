package openrouter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type staleRecoveryServer struct {
	completions atomic.Int32
	keyChecks   atomic.Int32
	lateStatus  int
	lateBody    string
	lateRetry   string

	firstStarted chan struct{}
	lateStarted  chan struct{}
	probeStarted chan struct{}
	releaseFirst chan struct{}
	releaseLate  chan struct{}
	releaseProbe chan struct{}
	releaseOnce  [3]sync.Once
}

func newStaleRecoveryServer() *staleRecoveryServer {
	return &staleRecoveryServer{
		lateStatus:   http.StatusPaymentRequired,
		lateBody:     `{"error":{"code":402}}`,
		firstStarted: make(chan struct{}),
		lateStarted:  make(chan struct{}),
		probeStarted: make(chan struct{}),
		releaseFirst: make(chan struct{}),
		releaseLate:  make(chan struct{}),
		releaseProbe: make(chan struct{}),
	}
}

func (s *staleRecoveryServer) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.URL.Path == "/v1/key" {
		s.keyChecks.Add(1)
		_, _ = io.WriteString(writer, `{"data":{"limit_remaining":1}}`)
		return
	}
	if request.URL.Path != "/alpha/decisions" {
		writer.WriteHeader(http.StatusNotFound)
		return
	}

	switch s.completions.Add(1) {
	case 1:
		close(s.firstStarted)
		<-s.releaseFirst
		writer.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(writer, `{"error":{"code":402}}`)
	case 2:
		close(s.lateStarted)
		<-s.releaseLate
		if s.lateRetry != "" {
			writer.Header().Set("Retry-After", s.lateRetry)
		}
		writer.WriteHeader(s.lateStatus)
		_, _ = io.WriteString(writer, s.lateBody)
	case 3:
		close(s.probeStarted)
		<-s.releaseProbe
		_, _ = io.WriteString(writer, completionJSON("recovery works"))
	default:
		_, _ = io.WriteString(writer, completionJSON("normal works"))
	}
}

func (s *staleRecoveryServer) release(index int, channel chan struct{}) {
	s.releaseOnce[index].Do(func() { close(channel) })
}

func (s *staleRecoveryServer) releaseAll() {
	s.release(0, s.releaseFirst)
	s.release(1, s.releaseLate)
	s.release(2, s.releaseProbe)
}

type completionResult struct {
	choice int
	err    error
}

func startCompletion(client *Client) <-chan completionResult {
	result := make(chan completionResult, 1)
	go func() {
		choice, err := client.Complete(context.Background(), validInput())
		result <- completionResult{choice: choice, err: err}
	}()
	return result
}

func awaitSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("%s did not start", name)
	}
}

func awaitCompletion(t *testing.T, result <-chan completionResult, name string) completionResult {
	t.Helper()
	select {
	case value := <-result:
		return value
	case <-time.After(time.Second):
		t.Fatalf("%s did not finish", name)
		return completionResult{}
	}
}

func TestStaleCreditFaultBeforeRecoverySuccessPreservesOwner(t *testing.T) {
	state := newStaleRecoveryServer()
	server := httptest.NewServer(state)
	t.Cleanup(func() {
		state.releaseAll()
		server.Close()
	})
	clock := newTestClock(time.Unix(1_000, 0))
	client, err := New(Config{
		APIKey: "test-key", Model: ModelJev,
		BalanceCooldown: time.Second, RequestTimeout: 5 * time.Second,
	}, WithBaseURL(server.URL), WithClock(clock.Now))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first := startCompletion(client)
	awaitSignal(t, state.firstStarted, "first completion")
	late := startCompletion(client)
	awaitSignal(t, state.lateStarted, "late completion")
	state.release(0, state.releaseFirst)
	if got := awaitCompletion(t, first, "first completion"); SafeFaultClass(got.err) != FaultCredit {
		t.Fatalf("first fault = %q, want credit", SafeFaultClass(got.err))
	}

	clock.Add(time.Second)
	recovery := startCompletion(client)
	awaitSignal(t, state.probeStarted, "recovery probe")
	state.release(1, state.releaseLate)
	if got := awaitCompletion(t, late, "late completion"); SafeFaultClass(got.err) != FaultCredit {
		t.Fatalf("late fault = %q, want credit", SafeFaultClass(got.err))
	}

	clock.Add(time.Second)
	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
		t.Fatalf("concurrent recovery fault = %q, want credit", SafeFaultClass(err))
	}
	if state.keyChecks.Load() != 1 || state.completions.Load() != 3 {
		t.Fatalf("active calls: key = %d, completion = %d", state.keyChecks.Load(), state.completions.Load())
	}

	state.release(2, state.releaseProbe)
	if got := awaitCompletion(t, recovery, "recovery probe"); got.err != nil || got.choice != 1 {
		t.Fatalf("recovery result = %d, %v", got.choice, got.err)
	}
	if choice, err := client.Complete(context.Background(), validInput()); err != nil || choice != 1 {
		t.Fatalf("normal completion = %d, %v", choice, err)
	}
	if state.keyChecks.Load() != 1 || state.completions.Load() != 4 {
		t.Fatalf("final calls: key = %d, completion = %d", state.keyChecks.Load(), state.completions.Load())
	}
}

func TestStaleCreditFaultAfterRecoverySuccessDoesNotReopenCircuit(t *testing.T) {
	state := newStaleRecoveryServer()
	server := httptest.NewServer(state)
	t.Cleanup(func() {
		state.releaseAll()
		server.Close()
	})
	clock := newTestClock(time.Unix(1_000, 0))
	client, err := New(Config{
		APIKey: "test-key", Model: ModelJev,
		BalanceCooldown: time.Second, RequestTimeout: 5 * time.Second,
	}, WithBaseURL(server.URL), WithClock(clock.Now))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	first := startCompletion(client)
	awaitSignal(t, state.firstStarted, "first completion")
	late := startCompletion(client)
	awaitSignal(t, state.lateStarted, "late completion")
	state.release(0, state.releaseFirst)
	if got := awaitCompletion(t, first, "first completion"); SafeFaultClass(got.err) != FaultCredit {
		t.Fatalf("first fault = %q, want credit", SafeFaultClass(got.err))
	}

	clock.Add(time.Second)
	recovery := startCompletion(client)
	awaitSignal(t, state.probeStarted, "recovery probe")
	state.release(2, state.releaseProbe)
	if got := awaitCompletion(t, recovery, "recovery probe"); got.err != nil || got.choice != 1 {
		t.Fatalf("recovery result = %d, %v", got.choice, got.err)
	}

	state.release(1, state.releaseLate)
	if got := awaitCompletion(t, late, "late completion"); SafeFaultClass(got.err) != FaultCredit {
		t.Fatalf("late fault = %q, want credit", SafeFaultClass(got.err))
	}
	if choice, err := client.Complete(context.Background(), validInput()); err != nil || choice != 1 {
		t.Fatalf("normal completion = %d, %v", choice, err)
	}
	if state.keyChecks.Load() != 1 || state.completions.Load() != 4 {
		t.Fatalf("final calls: key = %d, completion = %d", state.keyChecks.Load(), state.completions.Load())
	}
}
