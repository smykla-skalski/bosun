package openrouter

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCompleteRejectsUnsafeCandidatesBeforeNetwork(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return testResponse(http.StatusOK, completionJSON("unexpected"), ""), nil
	})}
	client, err := New(Config{
		APIKey: "test-key", Model: ModelGeminiFlashLite,
		BalanceCooldown: 15 * time.Minute, RequestTimeout: 5 * time.Second,
	}, WithBaseURL("https://openrouter.test"), WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	inputs := []Input{validInput(), validInput()}
	inputs[0].Candidates[2] = "Sprawdź https://private.example/path."
	inputs[1].Candidates[2] = inputs[1].Candidates[1]
	for _, input := range inputs {
		if _, err := client.Complete(context.Background(), input); SafeFaultClass(err) != FaultInvalidInput {
			t.Fatalf("Complete() fault = %q, want invalid input", SafeFaultClass(err))
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("remote requests = %d, want 0", requests.Load())
	}
}

func TestCompleteLimitsConcurrentModelRequests(t *testing.T) {
	t.Parallel()

	started := make(chan struct{}, maximumInFlight)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		started <- struct{}{}
		<-release
		_, _ = writer.Write([]byte(completionJSON("gotowe")))
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, time.Now)
	results := make(chan error, maximumInFlight)
	for range maximumInFlight {
		go func() {
			_, err := client.Complete(context.Background(), validInput())
			results <- err
		}()
	}
	for range maximumInFlight {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("a model request did not start")
		}
	}
	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultOverload {
		t.Fatalf("fifth Complete() fault = %q, want overload", SafeFaultClass(err))
	}
	close(release)
	for range maximumInFlight {
		if err := <-results; err != nil {
			t.Fatalf("model request error = %v", err)
		}
	}
}

func TestCompleteHonorsRequestTimeout(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	client, err := New(Config{
		APIKey: "test-key", Model: ModelGeminiFlashLite,
		BalanceCooldown: 15 * time.Minute, RequestTimeout: 20 * time.Millisecond,
	}, WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultTimeout {
		t.Fatalf("Complete() fault = %q, want timeout", SafeFaultClass(err))
	}
}
