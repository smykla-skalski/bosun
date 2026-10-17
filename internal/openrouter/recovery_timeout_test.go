package openrouter

import (
	"context"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecoveryRequestsReceiveIndependentTimeouts(t *testing.T) {
	const (
		requestTimeout = 500 * time.Millisecond
		requestDelay   = 300 * time.Millisecond
	)
	clock := newTestClock(time.Unix(1_000, 0))
	var requests atomic.Int32
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch requests.Add(1) {
		case 1:
			return testResponse(http.StatusPaymentRequired, `{"error":{"code":402}}`, ""), nil
		case 2:
			if request.URL.Path != "/v1/key" {
				t.Fatalf("second request path = %q, want /v1/key", request.URL.Path)
			}
			if err := waitForRequest(request, requestDelay); err != nil {
				return nil, err
			}
			return testResponse(http.StatusOK, `{"data":{"limit_remaining":1}}`, ""), nil
		case 3:
			if request.URL.Path != "/alpha/decisions" {
				t.Fatalf("third request path = %q, want /alpha/decisions", request.URL.Path)
			}
			if err := waitForRequest(request, requestDelay); err != nil {
				return nil, err
			}
			return testResponse(http.StatusOK, completionJSON("recovered"), ""), nil
		default:
			t.Fatalf("unexpected remote request %d", requests.Load())
			return nil, errors.New("unexpected remote request")
		}
	})}
	client := newTimeoutTestClient(t, httpClient, clock, requestTimeout)

	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
		t.Fatalf("initial fault = %q, want credit", SafeFaultClass(err))
	}
	clock.Add(15 * time.Minute)
	if choice, err := client.Complete(context.Background(), validInput()); err != nil || choice != 1 {
		t.Fatalf("recovery result = %d, %v", choice, err)
	}
	if requests.Load() != 3 {
		t.Fatalf("remote requests = %d, want 3", requests.Load())
	}
}

func TestRecoveryKeyCheckHonorsRequestTimeout(t *testing.T) {
	const requestTimeout = 20 * time.Millisecond
	clock := newTestClock(time.Unix(1_000, 0))
	var requests atomic.Int32
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch requests.Add(1) {
		case 1:
			return testResponse(http.StatusPaymentRequired, `{"error":{"code":402}}`, ""), nil
		case 2:
			if request.URL.Path != "/v1/key" {
				t.Fatalf("second request path = %q, want /v1/key", request.URL.Path)
			}
			<-request.Context().Done()
			return nil, request.Context().Err()
		default:
			t.Fatalf("unexpected remote request %d", requests.Load())
			return nil, errors.New("unexpected remote request")
		}
	})}
	client := newTimeoutTestClient(t, httpClient, clock, requestTimeout)

	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
		t.Fatalf("initial fault = %q, want credit", SafeFaultClass(err))
	}
	clock.Add(15 * time.Minute)
	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultTimeout {
		t.Fatalf("recovery fault = %q, want timeout", SafeFaultClass(err))
	}
	if requests.Load() != 2 {
		t.Fatalf("remote requests = %d, want 2", requests.Load())
	}
}

func newTimeoutTestClient(
	t *testing.T,
	httpClient *http.Client,
	clock *testClock,
	requestTimeout time.Duration,
) *Client {
	t.Helper()
	client, err := New(Config{
		APIKey: "test-key", Model: ModelJev,
		BalanceCooldown: 15 * time.Minute, RequestTimeout: requestTimeout,
	}, WithBaseURL("https://openrouter.test"), WithHTTPClient(httpClient), WithClock(clock.Now))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}

func waitForRequest(request *http.Request, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-request.Context().Done():
		return request.Context().Err()
	}
}
