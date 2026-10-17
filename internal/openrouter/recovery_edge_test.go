package openrouter

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestNullKeyLimitProbeReopensOnCreditError(t *testing.T) {
	t.Parallel()

	var completions atomic.Int32
	var keyChecks atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/key" {
			keyChecks.Add(1)
			_, _ = io.WriteString(writer, `{"data":{"limit_remaining":null}}`)
			return
		}
		completions.Add(1)
		writer.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(writer, `{"error":{"code":402}}`)
	}))
	defer server.Close()
	clock := newTestClock(time.Unix(1_000, 0))
	client := newTestClient(t, server.URL, clock.Now)

	_, _ = client.Complete(context.Background(), validInput())
	clock.Add(15 * time.Minute)
	for range 2 {
		if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
			t.Fatalf("probe fault = %q, want credit", SafeFaultClass(err))
		}
	}
	if keyChecks.Load() != 1 || completions.Load() != 2 {
		t.Fatalf("key checks = %d, completions = %d", keyChecks.Load(), completions.Load())
	}
}

func TestEmptyKeyLimitReopensCreditCircuit(t *testing.T) {
	t.Parallel()

	var completions atomic.Int32
	var keyChecks atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/key" {
			keyChecks.Add(1)
			_, _ = io.WriteString(writer, `{"data":{"limit_remaining":0}}`)
			return
		}
		completions.Add(1)
		writer.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(writer, `{"error":{"code":402}}`)
	}))
	defer server.Close()
	clock := newTestClock(time.Unix(1_000, 0))
	client := newTestClient(t, server.URL, clock.Now)

	_, _ = client.Complete(context.Background(), validInput())
	clock.Add(15 * time.Minute)
	for range 2 {
		if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
			t.Fatalf("recovery fault = %q, want credit", SafeFaultClass(err))
		}
	}
	if keyChecks.Load() != 1 || completions.Load() != 1 {
		t.Fatalf("key checks = %d, completions = %d", keyChecks.Load(), completions.Load())
	}
}

func TestMalformedKeyLimitReopensCreditCircuit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
	}{
		{name: "missing data", body: `{}`},
		{name: "null data", body: `{"data":null}`},
		{name: "missing limit", body: `{"data":{}}`},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			clock := newTestClock(time.Unix(1_000, 0))
			httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
				switch requests.Add(1) {
				case 1:
					return testResponse(http.StatusPaymentRequired, `{"error":{"code":402}}`, ""), nil
				case 2:
					if request.URL.Path != "/v1/key" {
						t.Fatalf("second request path = %q, want /v1/key", request.URL.Path)
					}
					return testResponse(http.StatusOK, test.body, ""), nil
				default:
					t.Fatalf("unexpected remote request %d", requests.Load())
					return nil, nil
				}
			})}
			client, err := New(Config{
				APIKey: "test-key", Model: ModelJev, BalanceCooldown: 15 * time.Minute, RequestTimeout: 5 * time.Second,
			}, WithBaseURL("https://openrouter.test"), WithHTTPClient(httpClient), WithClock(clock.Now))
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
				t.Fatalf("initial fault = %q, want credit", SafeFaultClass(err))
			}
			clock.Add(15 * time.Minute)
			if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultRemote {
				t.Fatalf("key fault = %q, want remote", SafeFaultClass(err))
			}
			if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
				t.Fatalf("reopened fault = %q, want credit", SafeFaultClass(err))
			}
			if requests.Load() != 2 {
				t.Fatalf("remote requests = %d, want 2", requests.Load())
			}
		})
	}
}
