package openrouter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestForbiddenInFlightBudgetUsesRetryAfterOnly(t *testing.T) {
	t.Parallel()

	var completions atomic.Int32
	var keyChecks atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/key" {
			keyChecks.Add(1)
		}
		if completions.Add(1) == 1 {
			writer.Header().Set("Retry-After", "10")
			writer.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(writer, `{"error":{"code":403,"metadata":{"limit_source":"openrouter_in_flight_budget"}}}`)
			return
		}
		_, _ = io.WriteString(writer, completionJSON("ponownie działa"))
	}))
	defer server.Close()
	clock := newTestClock(time.Unix(1_000, 0))
	client := newTestClient(t, server.URL, clock.Now)

	_, firstErr := client.Complete(context.Background(), validInput())
	_, deniedErr := client.Complete(context.Background(), validInput())
	if SafeFaultClass(firstErr) != FaultTemporaryCredit || SafeFaultClass(deniedErr) != FaultTemporaryCredit {
		t.Fatalf("temporary faults = %q, %q", SafeFaultClass(firstErr), SafeFaultClass(deniedErr))
	}
	clock.Add(10 * time.Second)
	choice, err := client.Complete(context.Background(), validInput())
	if err != nil || choice != 1 {
		t.Fatalf("Complete() after Retry-After = %d, %v", choice, err)
	}
	if completions.Load() != 2 || keyChecks.Load() != 0 {
		t.Fatalf("completion calls = %d, key checks = %d", completions.Load(), keyChecks.Load())
	}
}

func TestTemporaryHTTPStatusUsesRetryAfterDenyPeriod(t *testing.T) {
	t.Parallel()

	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		status := status
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				if requests.Add(1) == 1 {
					writer.Header().Set("Retry-After", "7")
					writer.WriteHeader(status)
					_, _ = io.WriteString(writer, `{"error":{"code":`+strconv.Itoa(status)+`}}`)
					return
				}
				_, _ = io.WriteString(writer, completionJSON("usługa wróciła"))
			}))
			defer server.Close()
			clock := newTestClock(time.Unix(1_000, 0))
			client := newTestClient(t, server.URL, clock.Now)

			_, firstErr := client.Complete(context.Background(), validInput())
			_, deniedErr := client.Complete(context.Background(), validInput())
			if SafeFaultClass(firstErr) != FaultTemporary || SafeFaultClass(deniedErr) != FaultTemporary {
				t.Fatalf("temporary faults = %q, %q", SafeFaultClass(firstErr), SafeFaultClass(deniedErr))
			}
			clock.Add(7 * time.Second)
			choice, err := client.Complete(context.Background(), validInput())
			if err != nil || choice != 1 {
				t.Fatalf("Complete() after Retry-After = %d, %v", choice, err)
			}
			if requests.Load() != 2 {
				t.Fatalf("remote requests = %d, want 2", requests.Load())
			}
		})
	}
}

func TestTemporaryHTTPStatusUsesDefaultDenyPeriodWithoutValidRetryAfter(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		retryAfter string
	}{
		{name: "429 without Retry-After", status: http.StatusTooManyRequests},
		{name: "503 with invalid Retry-After", status: http.StatusServiceUnavailable, retryAfter: "later"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				if requests.Add(1) == 1 {
					if test.retryAfter != "" {
						writer.Header().Set("Retry-After", test.retryAfter)
					}
					writer.WriteHeader(test.status)
					_, _ = io.WriteString(writer, `{"error":{"code":`+strconv.Itoa(test.status)+`}}`)
					return
				}
				_, _ = io.WriteString(writer, completionJSON("usługa wróciła"))
			}))
			defer server.Close()
			clock := newTestClock(time.Unix(1_000, 0))
			client := newTestClient(t, server.URL, clock.Now)

			if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultTemporary {
				t.Fatalf("initial fault = %q, want temporary", SafeFaultClass(err))
			}
			clock.Add(defaultRecoveryRetryDelay - time.Nanosecond)
			if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultTemporary {
				t.Fatalf("deny fault = %q, want temporary", SafeFaultClass(err))
			}
			if requests.Load() != 1 {
				t.Fatalf("remote requests before expiry = %d, want 1", requests.Load())
			}
			clock.Add(time.Nanosecond)
			choice, err := client.Complete(context.Background(), validInput())
			if err != nil || choice != 1 {
				t.Fatalf("Complete() after expiry = %d, %v", choice, err)
			}
			if requests.Load() != 2 {
				t.Fatalf("remote requests after expiry = %d, want 2", requests.Load())
			}
		})
	}
}

func TestTemporaryStatusPreservesRetryAfterWithUnreadableBody(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body func() io.ReadCloser
	}{
		{name: "invalid JSON", body: func() io.ReadCloser { return io.NopCloser(strings.NewReader("not-json")) }},
		{name: "oversize", body: func() io.ReadCloser {
			return io.NopCloser(strings.NewReader(strings.Repeat("x", maximumResponseBytes+1)))
		}},
		{name: "read fault", body: func() io.ReadCloser { return errorReadCloser{} }},
	}
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		for _, test := range tests {
			test := test
			t.Run(http.StatusText(status)+"/"+test.name, func(t *testing.T) {
				t.Parallel()
				runUnreadableTemporaryStatusTest(t, status, test.body)
			})
		}
	}
}

func runUnreadableTemporaryStatusTest(t *testing.T, status int, body func() io.ReadCloser) {
	t.Helper()

	var requests atomic.Int32
	clock := newTestClock(time.Unix(1_000, 0))
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		if requests.Add(1) == 1 {
			response := &http.Response{StatusCode: status, Header: make(http.Header), Body: body()}
			response.Header.Set("Retry-After", "7")
			return response, nil
		}
		return testResponse(http.StatusOK, completionJSON("usługa wróciła"), ""), nil
	})}
	client, err := New(Config{
		APIKey: "test-key", Model: ModelGeminiFlashLite, BalanceCooldown: 15 * time.Minute, RequestTimeout: 5 * time.Second,
	}, WithBaseURL("https://openrouter.test"), WithHTTPClient(httpClient), WithClock(clock.Now))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	_, firstErr := client.Complete(context.Background(), validInput())
	_, deniedErr := client.Complete(context.Background(), validInput())
	if SafeFaultClass(firstErr) != FaultTemporary || SafeFaultClass(deniedErr) != FaultTemporary || requests.Load() != 1 {
		t.Fatalf("faults = %q, %q; requests = %d", SafeFaultClass(firstErr), SafeFaultClass(deniedErr), requests.Load())
	}
	clock.Add(7 * time.Second)
	if choice, completeErr := client.Complete(context.Background(), validInput()); completeErr != nil || choice != 1 {
		t.Fatalf("Complete() after Retry-After = %d, %v", choice, completeErr)
	}
	if requests.Load() != 2 {
		t.Fatalf("remote requests = %d, want 2", requests.Load())
	}
}

func TestTemporaryBodyErrorUsesRetryAfterDenyPeriod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		code string
	}{
		{name: "numeric 429", code: `429`},
		{name: "string 503", code: `"503"`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				if requests.Add(1) == 1 {
					writer.Header().Set("Retry-After", "6")
					_, _ = io.WriteString(writer, `{"error":{"code":`+test.code+`}}`)
					return
				}
				_, _ = io.WriteString(writer, completionJSON("usługa wróciła"))
			}))
			defer server.Close()
			clock := newTestClock(time.Unix(1_000, 0))
			client := newTestClient(t, server.URL, clock.Now)

			_, firstErr := client.Complete(context.Background(), validInput())
			_, deniedErr := client.Complete(context.Background(), validInput())
			if SafeFaultClass(firstErr) != FaultTemporary || SafeFaultClass(deniedErr) != FaultTemporary {
				t.Fatalf("temporary faults = %q, %q", SafeFaultClass(firstErr), SafeFaultClass(deniedErr))
			}
			if requests.Load() != 1 {
				t.Fatalf("remote requests before expiry = %d, want 1", requests.Load())
			}
			clock.Add(6 * time.Second)
			choice, err := client.Complete(context.Background(), validInput())
			if err != nil || choice != 1 {
				t.Fatalf("Complete() after Retry-After = %d, %v", choice, err)
			}
			if requests.Load() != 2 {
				t.Fatalf("remote requests after expiry = %d, want 2", requests.Load())
			}
		})
	}
}
