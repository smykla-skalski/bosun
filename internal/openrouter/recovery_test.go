package openrouter

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestRecoveryKeyTemporaryFaultUsesBoundedDenyPeriod(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		status     int
		retryAfter string
		wantDelay  time.Duration
	}{
		{name: "429 without Retry-After", status: http.StatusTooManyRequests, wantDelay: defaultRecoveryRetryDelay},
		{name: "503 with zero Retry-After", status: http.StatusServiceUnavailable, retryAfter: "0", wantDelay: defaultRecoveryRetryDelay},
		{name: "longer valid delay", status: http.StatusTooManyRequests, retryAfter: "12", wantDelay: 12 * time.Second},
		{name: "bounded valid delay", status: http.StatusServiceUnavailable, retryAfter: "3600", wantDelay: maximumRetryDelay},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			var completions atomic.Int32
			var keyChecks atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch request.URL.Path {
				case "/alpha/decisions":
					if completions.Add(1) == 1 {
						writer.WriteHeader(http.StatusPaymentRequired)
						_, _ = io.WriteString(writer, `{"error":{"code":402}}`)
						return
					}
					_, _ = io.WriteString(writer, completionJSON("usługa wróciła"))
				case "/v1/key":
					if keyChecks.Add(1) == 1 {
						if test.retryAfter != "" {
							writer.Header().Set("Retry-After", test.retryAfter)
						}
						writer.WriteHeader(test.status)
						_, _ = io.WriteString(writer, `{"error":{"code":`+strconv.Itoa(test.status)+`}}`)
						return
					}
					_, _ = io.WriteString(writer, `{"data":{"limit_remaining":1}}`)
				default:
					writer.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			clock := newTestClock(time.Unix(1_000, 0))
			client := newTestClient(t, server.URL, clock.Now)

			if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
				t.Fatalf("initial fault = %q, want credit", SafeFaultClass(err))
			}
			clock.Add(15 * time.Minute)
			if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultTemporary {
				t.Fatalf("key check fault = %q, want temporary", SafeFaultClass(err))
			}
			clock.Add(test.wantDelay - time.Nanosecond)
			if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultTemporary {
				t.Fatalf("deny fault = %q, want temporary", SafeFaultClass(err))
			}
			if keyChecks.Load() != 1 || completions.Load() != 1 {
				t.Fatalf("calls before expiry: key = %d, completion = %d", keyChecks.Load(), completions.Load())
			}
			clock.Add(time.Nanosecond)
			choice, err := client.Complete(context.Background(), validInput())
			if err != nil || choice != 1 {
				t.Fatalf("Complete() after expiry = %d, %v", choice, err)
			}
			if keyChecks.Load() != 2 || completions.Load() != 2 {
				t.Fatalf("calls after expiry: key = %d, completion = %d", keyChecks.Load(), completions.Load())
			}
		})
	}
}

func TestInFlightBudgetUsesOnlyRetryAfterDenyPeriod(t *testing.T) {
	t.Parallel()

	var completions atomic.Int32
	var keyChecks atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/key" {
			keyChecks.Add(1)
		}
		if completions.Add(1) == 1 {
			writer.Header().Set("Retry-After", "10")
			writer.WriteHeader(http.StatusPaymentRequired)
			_, _ = io.WriteString(writer, `{"error":{"code":"402","metadata":{"limit_source":"openrouter_in_flight_budget"}}}`)
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

func TestPositiveKeyLimitKeepsOneRecoveryProbeUnderConcurrency(t *testing.T) {
	t.Parallel()

	var completions atomic.Int32
	var keyChecks atomic.Int32
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/alpha/decisions":
			switch completions.Add(1) {
			case 1:
				writer.WriteHeader(http.StatusPaymentRequired)
				_, _ = io.WriteString(writer, `{"error":{"code":402,"metadata":{"limit_source":"openrouter_account_balance"}}}`)
				return
			case 2:
				close(probeStarted)
				<-releaseProbe
			}
			_, _ = io.WriteString(writer, completionJSON("kredyt wrócił"))
		case "/v1/key":
			keyChecks.Add(1)
			_, _ = io.WriteString(writer, `{"data":{"limit_remaining":1.25}}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	clock := newTestClock(time.Unix(1_000, 0))
	client := newTestClient(t, server.URL, clock.Now)

	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
		t.Fatalf("first Complete() fault = %q", SafeFaultClass(err))
	}
	clock.Add(15 * time.Minute)
	leader := make(chan error, 1)
	go func() {
		_, err := client.Complete(context.Background(), validInput())
		leader <- err
	}()
	select {
	case <-probeStarted:
	case <-time.After(time.Second):
		t.Fatal("recovery completion probe did not start")
	}

	const followers = 12
	followerResults := make(chan error, followers)
	for range followers {
		go func() {
			_, err := client.Complete(context.Background(), validInput())
			followerResults <- err
		}()
	}
	for range followers {
		select {
		case err := <-followerResults:
			if SafeFaultClass(err) != FaultCredit {
				t.Fatalf("follower fault = %q, want credit", SafeFaultClass(err))
			}
		case <-time.After(time.Second):
			t.Fatal("follower waited for the completion probe")
		}
	}
	if keyChecks.Load() != 1 || completions.Load() != 2 {
		t.Fatalf("active probe calls: key = %d, completion = %d", keyChecks.Load(), completions.Load())
	}
	close(releaseProbe)
	if err := <-leader; err != nil {
		t.Fatalf("recovery Complete() error = %v", err)
	}
	if choice, err := client.Complete(context.Background(), validInput()); err != nil || choice != 1 {
		t.Fatalf("Complete() after recovery = %d, %v", choice, err)
	}
	if keyChecks.Load() != 1 || completions.Load() != 3 {
		t.Fatalf("key checks = %d, completions = %d", keyChecks.Load(), completions.Load())
	}
}

func TestNullKeyLimitAllowsOneCompletionProbe(t *testing.T) {
	t.Parallel()

	var completions atomic.Int32
	var keyChecks atomic.Int32
	probeStarted := make(chan struct{})
	releaseProbe := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path == "/v1/key" {
			keyChecks.Add(1)
			_, _ = io.WriteString(writer, `{"data":{"limit_remaining":null}}`)
			return
		}
		switch completions.Add(1) {
		case 1:
			writer.WriteHeader(http.StatusPaymentRequired)
			_, _ = io.WriteString(writer, `{"error":{"code":402}}`)
			return
		case 2:
			close(probeStarted)
			<-releaseProbe
		}
		_, _ = io.WriteString(writer, completionJSON("próba działa"))
	}))
	defer server.Close()
	clock := newTestClock(time.Unix(1_000, 0))
	client := newTestClient(t, server.URL, clock.Now)

	_, _ = client.Complete(context.Background(), validInput())
	clock.Add(15 * time.Minute)
	probeResult := make(chan struct {
		choice int
		err    error
	}, 1)
	go func() {
		choice, err := client.Complete(context.Background(), validInput())
		probeResult <- struct {
			choice int
			err    error
		}{choice: choice, err: err}
	}()
	select {
	case <-probeStarted:
	case <-time.After(time.Second):
		t.Fatal("completion probe did not start")
	}
	for range 4 {
		if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
			t.Fatalf("concurrent probe fault = %q, want credit", SafeFaultClass(err))
		}
	}
	close(releaseProbe)
	result := <-probeResult
	if result.err != nil || result.choice != 1 {
		t.Fatalf("probe Complete() = %d, %v", result.choice, result.err)
	}
	choice, err := client.Complete(context.Background(), validInput())
	if err != nil || choice != 1 {
		t.Fatalf("Complete() after probe = %d, %v", choice, err)
	}
	if keyChecks.Load() != 1 || completions.Load() != 3 {
		t.Fatalf("key checks = %d, completions = %d", keyChecks.Load(), completions.Load())
	}
}

func TestNullKeyLimitProbeFailureReopensCreditCircuit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		probe       func() (*http.Response, error)
		wantFault   string
		retryPeriod time.Duration
	}{
		{
			name: "timeout",
			probe: func() (*http.Response, error) {
				return nil, context.DeadlineExceeded
			},
			wantFault: FaultTimeout,
		},
		{
			name: "transport fault",
			probe: func() (*http.Response, error) {
				return nil, errors.New("test transport fault")
			},
			wantFault: FaultTransport,
		},
		{
			name: "invalid output",
			probe: func() (*http.Response, error) {
				return testResponse(http.StatusOK, `{"choices":[]}`, ""), nil
			},
			wantFault: FaultInvalidOutput,
		},
		{
			name: "HTTP 429",
			probe: func() (*http.Response, error) {
				return testResponse(http.StatusTooManyRequests, `{"error":{"code":429}}`, "6"), nil
			},
			wantFault:   FaultTemporary,
			retryPeriod: 6 * time.Second,
		},
		{
			name: "body 503",
			probe: func() (*http.Response, error) {
				return testResponse(http.StatusOK, `{"error":{"code":503}}`, "6"), nil
			},
			wantFault:   FaultTemporary,
			retryPeriod: 6 * time.Second,
		},
		{
			name: "other remote fault",
			probe: func() (*http.Response, error) {
				return testResponse(http.StatusInternalServerError, `{"error":{"code":500}}`, ""), nil
			},
			wantFault: FaultRemote,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			runNullKeyLimitProbeFailureTest(t, test.probe, test.wantFault, test.retryPeriod)
		})
	}
}

func runNullKeyLimitProbeFailureTest(t *testing.T, probe func() (*http.Response, error), wantFault string, retryPeriod time.Duration) {
	t.Helper()
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
			return testResponse(http.StatusOK, `{"data":{"limit_remaining":null}}`, ""), nil
		case 3:
			return probe()
		default:
			t.Fatalf("unexpected remote request %d", requests.Load())
			return nil, errors.New("unexpected remote request")
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
	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != wantFault {
		t.Fatalf("probe fault = %q, want %q", SafeFaultClass(err), wantFault)
	}
	if retryPeriod > 0 {
		if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultTemporary {
			t.Fatalf("local deny fault = %q, want temporary", SafeFaultClass(err))
		}
		clock.Add(retryPeriod)
	}
	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
		t.Fatalf("reopened circuit fault = %q, want credit", SafeFaultClass(err))
	}
	if requests.Load() != 3 {
		t.Fatalf("remote requests = %d, want 3", requests.Load())
	}
}

func TestNullKeyLimitInFlightProbeKeepsTemporaryRecovery(t *testing.T) {
	t.Parallel()

	var completions atomic.Int32
	var keyChecks atomic.Int32
	recoveryStarted := make(chan struct{})
	releaseRecovery := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/alpha/decisions":
			switch completions.Add(1) {
			case 1:
				writer.WriteHeader(http.StatusPaymentRequired)
				_, _ = io.WriteString(writer, `{"error":{"code":402}}`)
			case 2:
				writer.Header().Set("Retry-After", "7")
				writer.WriteHeader(http.StatusPaymentRequired)
				_, _ = io.WriteString(writer, `{"error":{"code":402,"metadata":{"limit_source":"openrouter_in_flight_budget"}}}`)
			default:
				_, _ = io.WriteString(writer, completionJSON("próba działa"))
			}
		case "/v1/key":
			if keyChecks.Add(1) == 2 {
				close(recoveryStarted)
				<-releaseRecovery
			}
			_, _ = io.WriteString(writer, `{"data":{"limit_remaining":null}}`)
		default:
			writer.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	clock := newTestClock(time.Unix(1_000, 0))
	client := newTestClient(t, server.URL, clock.Now)

	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
		t.Fatalf("initial fault = %q, want credit", SafeFaultClass(err))
	}
	clock.Add(15 * time.Minute)
	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultTemporaryCredit {
		t.Fatalf("probe fault = %q, want temporary credit", SafeFaultClass(err))
	}
	for range 4 {
		if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultTemporaryCredit {
			t.Fatalf("temporary deny fault = %q, want temporary credit", SafeFaultClass(err))
		}
	}
	if keyChecks.Load() != 1 || completions.Load() != 2 {
		t.Fatalf("calls during delay: key = %d, completion = %d", keyChecks.Load(), completions.Load())
	}

	clock.Add(7 * time.Second)
	leader := make(chan struct {
		choice int
		err    error
	}, 1)
	go func() {
		choice, err := client.Complete(context.Background(), validInput())
		leader <- struct {
			choice int
			err    error
		}{choice: choice, err: err}
	}()
	select {
	case <-recoveryStarted:
	case <-time.After(time.Second):
		t.Fatal("shared recovery check did not start")
	}

	const followers = 8
	results := make(chan error, followers)
	for range followers {
		go func() {
			_, err := client.Complete(context.Background(), validInput())
			results <- err
		}()
	}
	for range followers {
		select {
		case err := <-results:
			if SafeFaultClass(err) != FaultCredit {
				t.Fatalf("concurrent recovery fault = %q, want credit", SafeFaultClass(err))
			}
		case <-time.After(time.Second):
			t.Fatal("concurrent request waited for recovery")
		}
	}
	close(releaseRecovery)
	result := <-leader
	if result.err != nil || result.choice != 1 {
		t.Fatalf("recovery result = %d, %v", result.choice, result.err)
	}
	if keyChecks.Load() != 2 || completions.Load() != 3 {
		t.Fatalf("calls after recovery: key = %d, completion = %d", keyChecks.Load(), completions.Load())
	}
}
