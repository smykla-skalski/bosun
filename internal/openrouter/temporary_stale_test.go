package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

type staleTemporaryCase struct {
	name      string
	status    int
	body      string
	wantFault string
}

var staleTemporaryCases = []staleTemporaryCase{
	{name: "HTTP 429", status: http.StatusTooManyRequests, body: `{"error":{"code":429}}`, wantFault: FaultTemporary},
	{name: "HTTP 503", status: http.StatusServiceUnavailable, body: `{"error":{"code":503}}`, wantFault: FaultTemporary},
	{name: "body 429", status: http.StatusOK, body: `{"error":{"code":429}}`, wantFault: FaultTemporary},
	{name: "body 503", status: http.StatusOK, body: `{"error":{"code":503}}`, wantFault: FaultTemporary},
	{
		name: "in-flight budget", status: http.StatusForbidden,
		body:      `{"error":{"code":403,"metadata":{"limit_source":"openrouter_in_flight_budget"}}}`,
		wantFault: FaultTemporaryCredit,
	},
}

func TestStaleTemporaryFaultBeforeRecoverySuccessClearsOldDelay(t *testing.T) {
	for _, test := range staleTemporaryCases {
		t.Run(test.name, func(t *testing.T) {
			state, client, clock := newStaleTemporaryClient(t, test)
			first, late := startTwoStaleCompletions(t, state, client)
			state.release(0, state.releaseFirst)
			assertCompletionFault(t, first, FaultCredit, "first completion")

			clock.Add(time.Second)
			recovery := startCompletion(client)
			awaitSignal(t, state.probeStarted, "recovery probe")
			state.release(1, state.releaseLate)
			assertCompletionFault(t, late, test.wantFault, "late completion")
			if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != test.wantFault {
				t.Fatalf("active delay fault = %q, want %q", SafeFaultClass(err), test.wantFault)
			}
			state.release(2, state.releaseProbe)
			assertCompletionChoice(t, recovery, "recovery probe")

			assertDirectCompletion(t, client)
			assertStaleCallCounts(t, state, 4)
		})
	}
}

func TestStaleTemporaryFaultAfterRecoverySuccessDoesNotDelay(t *testing.T) {
	for _, test := range staleTemporaryCases {
		t.Run(test.name, func(t *testing.T) {
			state, client, clock := newStaleTemporaryClient(t, test)
			first, late := startTwoStaleCompletions(t, state, client)
			state.release(0, state.releaseFirst)
			assertCompletionFault(t, first, FaultCredit, "first completion")

			clock.Add(time.Second)
			recovery := startCompletion(client)
			awaitSignal(t, state.probeStarted, "recovery probe")
			state.release(2, state.releaseProbe)
			assertCompletionChoice(t, recovery, "recovery probe")
			state.release(1, state.releaseLate)
			assertCompletionFault(t, late, test.wantFault, "late completion")

			assertDirectCompletion(t, client)
			assertStaleCallCounts(t, state, 4)
		})
	}
}

func newStaleTemporaryClient(
	t *testing.T,
	test staleTemporaryCase,
) (*staleRecoveryServer, *Client, *testClock) {
	t.Helper()
	state := newStaleRecoveryServer()
	state.lateStatus = test.status
	state.lateBody = test.body
	state.lateRetry = "10"
	server := httptest.NewServer(state)
	t.Cleanup(func() {
		state.releaseAll()
		server.Close()
	})
	clock := newTestClock(time.Unix(1_000, 0))
	client, err := New(Config{
		APIKey: "test-key", Model: ModelGeminiFlashLite,
		BalanceCooldown: time.Second, RequestTimeout: 5 * time.Second,
	}, WithBaseURL(server.URL), WithClock(clock.Now))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return state, client, clock
}

func startTwoStaleCompletions(
	t *testing.T,
	state *staleRecoveryServer,
	client *Client,
) (<-chan completionResult, <-chan completionResult) {
	t.Helper()
	first := startCompletion(client)
	awaitSignal(t, state.firstStarted, "first completion")
	late := startCompletion(client)
	awaitSignal(t, state.lateStarted, "late completion")
	return first, late
}

func assertCompletionFault(t *testing.T, result <-chan completionResult, want string, name string) {
	t.Helper()
	if got := awaitCompletion(t, result, name); SafeFaultClass(got.err) != want {
		t.Fatalf("%s fault = %q, want %q", name, SafeFaultClass(got.err), want)
	}
}

func assertCompletionChoice(t *testing.T, result <-chan completionResult, name string) {
	t.Helper()
	if got := awaitCompletion(t, result, name); got.err != nil || got.choice != 1 {
		t.Fatalf("%s result = %d, %v", name, got.choice, got.err)
	}
}

func assertDirectCompletion(t *testing.T, client *Client) {
	t.Helper()
	if choice, err := client.Complete(context.Background(), validInput()); err != nil || choice != 1 {
		t.Fatalf("normal completion = %d, %v", choice, err)
	}
}

func assertStaleCallCounts(t *testing.T, state *staleRecoveryServer, completions int32) {
	t.Helper()
	if state.keyChecks.Load() != 1 || state.completions.Load() != completions {
		t.Fatalf("final calls: key = %d, completion = %d", state.keyChecks.Load(), state.completions.Load())
	}
}
