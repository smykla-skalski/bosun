package openrouter

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smykla-skalski/bosun/internal/messageprofile"
	"github.com/smykla-skalski/bosun/internal/replies"
)

func TestCompleteSendsDecisionContractAndReturnsChoice(t *testing.T) {
	t.Parallel()

	canary := "Jan Kowalski @captain 123456789 https://private.example/secret DiScOrD.Gg/private-invite DiScOrD.CoM/InViTe/private-invite report.pdf Discord Bosun configured-server"
	profile, err := messageprofile.Build(canary, "Bosun", "ask", "request", "harsh")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input := validInput()
	input.Profile = profile
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		assertDecisionRequest(t, request, input)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, decisionJSON(6))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL+"/api", time.Now)
	choice, err := client.Complete(context.Background(), input)
	if err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if choice != 6 {
		t.Fatalf("Complete() = %d, want 6", choice)
	}
	if requests.Load() != 1 {
		t.Fatalf("model requests = %d, want 1", requests.Load())
	}
}

func assertDecisionRequest(t *testing.T, request *http.Request, input Input) {
	t.Helper()
	if request.Method != http.MethodPost || request.URL.Path != "/api/alpha/decisions" {
		t.Errorf("request = %s %s", request.Method, request.URL.Path)
	}
	if request.Header.Get("Authorization") != "Bearer test-key" || request.Header.Get("X-OpenRouter-Cache") != "false" {
		t.Error("request has incorrect authentication or cache headers")
	}
	for _, header := range []string{"HTTP-Referer", "X-OpenRouter-Title", "X-OpenRouter-Categories"} {
		if request.Header.Get(header) != "" {
			t.Errorf("request includes attribution header %q", header)
		}
	}
	headerText := ""
	for name, values := range request.Header {
		headerText += name + " " + strings.Join(values, " ") + "\n"
	}
	rawBody, err := io.ReadAll(request.Body)
	if err != nil {
		t.Errorf("read request: %v", err)
		return
	}
	assertNoCanaries(t, "headers", headerText)
	assertNoCanaries(t, "body", string(rawBody))
	var body map[string]json.RawMessage
	if err := json.Unmarshal(rawBody, &body); err != nil {
		t.Errorf("decode request: %v", err)
		return
	}
	assertExactKeys(t, body, "model", "state", "questions", "provider")
	assertRequestField(t, body, "model", ModelJev)
	var provider map[string]json.RawMessage
	if err := json.Unmarshal(body["provider"], &provider); err != nil {
		t.Errorf("decode provider: %v", err)
		return
	}
	assertExactKeys(t, provider, "zdr", "data_collection", "require_parameters")
	assertRequestField(t, provider, "zdr", true)
	assertRequestField(t, provider, "data_collection", "deny")
	assertRequestField(t, provider, "require_parameters", true)
	assertDecisionState(t, body["state"], input.Profile)
	assertDecisionQuestion(t, body["questions"], input.Candidates)
}

func assertDecisionState(t *testing.T, raw json.RawMessage, want messageprofile.Profile) {
	t.Helper()
	var got messageprofile.Profile
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if got != want {
		t.Errorf("state = %#v, want %#v", got, want)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil {
		t.Fatalf("decode state fields: %v", err)
	}
	assertExactKeys(t, fields,
		"version", "trigger", "category", "form", "structure", "length_bucket",
		"punctuation", "tone", "humor", "directness", "casing", "profanity_strength", "spice",
	)
}

func assertDecisionQuestion(t *testing.T, raw json.RawMessage, candidates [CandidateCount]string) {
	t.Helper()
	var questions map[string]json.RawMessage
	if err := json.Unmarshal(raw, &questions); err != nil {
		t.Fatalf("decode questions: %v", err)
	}
	assertExactKeys(t, questions, "reply")
	var question map[string]json.RawMessage
	if err := json.Unmarshal(questions["reply"], &question); err != nil {
		t.Fatalf("decode reply question: %v", err)
	}
	assertExactKeys(t, question, "type", "instructions", "criteria")
	assertRequestField(t, question, "type", "choice")
	assertRequestField(t, question, "instructions", decisionInstructions)

	var criteria map[string]json.RawMessage
	if err := json.Unmarshal(question["criteria"], &criteria); err != nil {
		t.Fatalf("decode criteria: %v", err)
	}
	if len(criteria) != CandidateCount {
		t.Fatalf("criteria count = %d, want %d", len(criteria), CandidateCount)
	}
	for index, candidate := range candidates {
		assertRequestField(t, criteria, candidateIDs[index], candidate)
		if err := replies.ValidateModelCandidate(candidate); err != nil {
			t.Errorf("candidate %d is unsafe: %v", index, err)
		}
	}
}

func assertNoCanaries(t *testing.T, location string, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, forbidden := range []string{"jan", "kowalski", "captain", "123456789", "private.example", "report.pdf", "discord.gg", "discord.com/invite", "discord", "bosun", "configured-server"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("request %s contains canary %q", location, forbidden)
		}
	}
}

func TestDecisionInstructionsKeepSafetyAndDataBoundaries(t *testing.T) {
	t.Parallel()

	for _, rule := range []string{
		"anonimowego profilu wiadomości",
		"Użyj wszystkich cech profilu",
		"drugie zdanie dodaje nowy obraz komiczny",
		"Nie preferuj próśb o dodatkowy kontekst",
	} {
		if !strings.Contains(decisionInstructions, rule) {
			t.Fatalf("decision instructions do not contain rule %q", rule)
		}
	}
	for _, forbidden := range []string{"discord", "bosun", "configured-server"} {
		if strings.Contains(strings.ToLower(decisionInstructions), forbidden) {
			t.Fatalf("decision instructions contain forbidden term %q", forbidden)
		}
	}
}

func TestNewRejectsEveryModelOutsideAllowlist(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"", "example/model", "private-person/private-model", "example/discord-model", "configured-server/model"} {
		_, err := New(Config{
			APIKey: "test-key", Model: model, BalanceCooldown: 15 * time.Minute, RequestTimeout: 5 * time.Second,
		})
		if err == nil {
			t.Errorf("New() accepted model %q", model)
		}
		if strings.Contains(err.Error(), model) && model != "" {
			t.Errorf("New() error contains rejected model %q", model)
		}
	}
}

func TestCompleteRejectsEveryNonDecisionOutput(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
	}{
		{name: "missing", content: `{}`},
		{name: "extra", content: `{"choice":0,"extra":true}`},
		{name: "fractional", content: `{"choice":1.0}`},
		{name: "string", content: `{"choice":"1"}`},
		{name: "negative", content: `{"choice":-1}`},
		{name: "out of range", content: `{"choice":8}`},
		{name: "duplicate key", content: `{"choice":0,"choice":1}`},
		{name: "trailing data", content: `{"choice":0} {"choice":1}`},
		{name: "threat text", content: `Jutro cię zabiję.`},
		{name: "contact text", content: `Mój numer to 501234567.`},
		{name: "English text", content: `You failed. Try again.`},
		{name: "malformed", content: `{"choice":`},
		{name: "blank", content: ` `},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(writer, rawDecisionResponse(test.content))
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, time.Now)
			choice, err := client.Complete(context.Background(), validInput())
			if choice != 0 || SafeFaultClass(err) != FaultInvalidOutput {
				t.Fatalf("Complete() = %d, %q; want candidate zero fault", choice, SafeFaultClass(err))
			}
		})
	}
}

func TestCompleteAcceptsBoundaryDecisions(t *testing.T) {
	t.Parallel()

	for _, choice := range []int{0, CandidateCount - 1} {
		choice := choice
		t.Run(string(rune('0'+choice)), func(t *testing.T) {
			t.Parallel()
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = io.WriteString(writer, decisionJSON(choice))
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, time.Now)
			got, err := client.Complete(context.Background(), validInput())
			if err != nil || got != choice {
				t.Fatalf("Complete() = %d, %v; want %d", got, err, choice)
			}
		})
	}
}

func TestCompleteRejectsPartialChoiceWithError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"answers":{"reply":{"type":"choice","choice":"candidate_1","probabilities":{},"confidence":1}},"error":{"code":500}}`)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, time.Now)

	if choice, err := client.Complete(context.Background(), validInput()); choice != 0 || SafeFaultClass(err) != FaultRemote {
		t.Fatalf("Complete() = %d, %q; want remote fault", choice, SafeFaultClass(err))
	}
}

func TestCompleteAcceptsRoundedDecisionProbabilities(t *testing.T) {
	t.Parallel()

	probabilities := map[string]float64{
		"candidate_0": 0.13,
		"candidate_1": 0.12,
		"candidate_2": 0.12,
		"candidate_3": 0.12,
		"candidate_4": 0.12,
		"candidate_5": 0.14,
		"candidate_6": 0.12,
		"candidate_7": 0.12,
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, decisionJSONWithProbabilities(5, probabilities, 0.61))
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, time.Now)

	selected, err := client.Complete(context.Background(), validInput())
	if selected != 5 || err != nil {
		t.Fatalf("Complete() = %d, %v; want choice 5", selected, err)
	}
}

func TestFinalCreditErrorOpensCircuit(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		writer.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(writer, `{"error":{"code":402,"metadata":{"limit_source":"openrouter_account_balance"}}}`)
	}))
	defer server.Close()
	clock := newTestClock(time.Unix(1_000, 0))
	client := newTestClient(t, server.URL, clock.Now)

	for range 2 {
		_, err := client.Complete(context.Background(), validInput())
		if SafeFaultClass(err) != FaultCredit {
			t.Fatalf("Complete() fault = %q, want credit fault", SafeFaultClass(err))
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("remote requests = %d, want 1", requests.Load())
	}
}

func TestPaymentRequiredWithUnreadableBodyOpensCircuit(t *testing.T) {
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
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
				requests.Add(1)
				return &http.Response{StatusCode: http.StatusPaymentRequired, Header: make(http.Header), Body: test.body()}, nil
			})}
			client, err := New(Config{
				APIKey: "test-key", Model: ModelJev, BalanceCooldown: 15 * time.Minute, RequestTimeout: 5 * time.Second,
			}, WithBaseURL("https://openrouter.test"), WithHTTPClient(httpClient))
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}
			for range 2 {
				if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
					t.Fatalf("Complete() fault = %q, want credit", SafeFaultClass(err))
				}
			}
			if requests.Load() != 1 {
				t.Fatalf("remote requests = %d, want 1", requests.Load())
			}
		})
	}
}

func TestCompleteRejectsInvalidProfileBeforeNetwork(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		requests.Add(1)
		return testResponse(http.StatusOK, completionJSON("unexpected"), ""), nil
	})}
	client, err := New(Config{
		APIKey: "test-key", Model: ModelJev, BalanceCooldown: 15 * time.Minute, RequestTimeout: 5 * time.Second,
	}, WithBaseURL("https://openrouter.test"), WithHTTPClient(httpClient))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	input := validInput()
	input.Profile.Tone = "private free text"
	if _, err := client.Complete(context.Background(), input); SafeFaultClass(err) != FaultInvalidInput {
		t.Fatalf("Complete() fault = %q, want invalid input", SafeFaultClass(err))
	}
	if requests.Load() != 0 {
		t.Fatalf("remote requests = %d, want 0", requests.Load())
	}
}

func TestBodyCreditErrorOpensCircuit(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(writer, `{"error":{"code":"402","metadata":{"limit_source":"openrouter_account_balance"}}}`)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, time.Now)

	for range 2 {
		_, err := client.Complete(context.Background(), validInput())
		if SafeFaultClass(err) != FaultCredit {
			t.Fatalf("Complete() fault = %q, want credit fault", SafeFaultClass(err))
		}
	}
	if requests.Load() != 1 {
		t.Fatalf("remote requests = %d, want 1", requests.Load())
	}
}

func TestForbiddenBudgetFaultOpensCreditCircuit(t *testing.T) {
	t.Parallel()

	for _, limitSource := range []string{"openrouter_key_limit", "openrouter_account_balance"} {
		limitSource := limitSource
		t.Run(limitSource, func(t *testing.T) {
			t.Parallel()
			var requests atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				writer.WriteHeader(http.StatusForbidden)
				_, _ = io.WriteString(writer, `{"error":{"code":403,"metadata":{"limit_source":"`+limitSource+`"}}}`)
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, time.Now)

			for range 2 {
				_, err := client.Complete(context.Background(), validInput())
				if SafeFaultClass(err) != FaultCredit {
					t.Fatalf("Complete() fault = %q, want credit", SafeFaultClass(err))
				}
			}
			if requests.Load() != 1 {
				t.Fatalf("remote requests = %d, want 1", requests.Load())
			}
		})
	}
}

func TestBodyForbiddenBudgetFaultOpensCreditCircuit(t *testing.T) {
	t.Parallel()

	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = io.WriteString(writer, `{"error":{"code":403,"metadata":{"limit_source":"openrouter_key_limit"}}}`)
	}))
	defer server.Close()
	clock := newTestClock(time.Unix(1_000, 0))
	client := newTestClient(t, server.URL, clock.Now)

	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
		t.Fatalf("initial fault = %q, want credit", SafeFaultClass(err))
	}
	clock.Add(14*time.Minute + 59*time.Second)
	if _, err := client.Complete(context.Background(), validInput()); SafeFaultClass(err) != FaultCredit {
		t.Fatalf("cooldown fault = %q, want credit", SafeFaultClass(err))
	}
	if requests.Load() != 1 {
		t.Fatalf("remote requests = %d, want 1", requests.Load())
	}
}
