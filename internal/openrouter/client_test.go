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

	canary := "Jan Kowalski @captain 123456789 https://private.example/secret DiScOrD.Gg/private-invite DiScOrD.CoM/InViTe/private-invite report.pdf Discord Bosun smykland"
	profile, err := messageprofile.Build(canary, "Bosun", "ask", "request", "harsh")
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	input := validInput()
	input.Profile = profile
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests.Add(1)
		assertCompletionRequest(t, request, input)
		writer.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(writer, `{"choices":[{"message":{"content":"{\"choice\":6}"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()

	client := newTestClient(t, server.URL+"/api/v1", time.Now)
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

func assertCompletionRequest(t *testing.T, request *http.Request, profile Input) {
	t.Helper()
	if request.Method != http.MethodPost || request.URL.Path != "/api/v1/chat/completions" {
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
	assertExactKeys(t, body, "model", "messages", "stream", "max_completion_tokens", "reasoning_effort", "provider", "response_format")
	assertRequestField(t, body, "model", ModelGeminiFlashLite)
	assertRequestField(t, body, "stream", false)
	assertRequestField(t, body, "max_completion_tokens", float64(64))
	assertRequestField(t, body, "reasoning_effort", "minimal")
	var provider map[string]json.RawMessage
	if err := json.Unmarshal(body["provider"], &provider); err != nil {
		t.Errorf("decode provider: %v", err)
		return
	}
	assertExactKeys(t, provider, "zdr", "data_collection", "require_parameters")
	assertRequestField(t, provider, "zdr", true)
	assertRequestField(t, provider, "data_collection", "deny")
	assertRequestField(t, provider, "require_parameters", true)
	assertResponseFormat(t, body["response_format"])
	assertProfileMessage(t, body["messages"], profile)
}

func assertResponseFormat(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var format map[string]json.RawMessage
	if err := json.Unmarshal(raw, &format); err != nil {
		t.Fatalf("decode response format: %v", err)
	}
	assertExactKeys(t, format, "type", "json_schema")
	assertRequestField(t, format, "type", "json_schema")
	var contract map[string]json.RawMessage
	if err := json.Unmarshal(format["json_schema"], &contract); err != nil {
		t.Fatalf("decode JSON schema contract: %v", err)
	}
	assertExactKeys(t, contract, "name", "strict", "schema")
	assertRequestField(t, contract, "name", "reply_choice")
	assertRequestField(t, contract, "strict", true)
	assertChoiceSchema(t, contract["schema"])
}

func assertChoiceSchema(t *testing.T, raw json.RawMessage) {
	t.Helper()
	var schema map[string]json.RawMessage
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatalf("decode choice schema: %v", err)
	}
	assertExactKeys(t, schema, "type", "properties", "required", "additionalProperties")
	assertRequestField(t, schema, "type", "object")
	assertRequestField(t, schema, "additionalProperties", false)
	assertRequestField(t, schema, "required", [1]string{"choice"})
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(schema["properties"], &properties); err != nil {
		t.Fatalf("decode choice properties: %v", err)
	}
	assertExactKeys(t, properties, "choice")
	var choice map[string]json.RawMessage
	if err := json.Unmarshal(properties["choice"], &choice); err != nil {
		t.Fatalf("decode choice property: %v", err)
	}
	assertExactKeys(t, choice, "type", "description", "enum")
	assertRequestField(t, choice, "type", "integer")
	assertRequestField(t, choice, "description", "Indeks najlepiej dopasowanej riposty od 0 do 7.")
	assertRequestField(t, choice, "enum", [CandidateCount]int{0, 1, 2, 3, 4, 5, 6, 7})
}

func assertProfileMessage(t *testing.T, raw json.RawMessage, profile Input) {
	t.Helper()
	var messages []message
	if err := json.Unmarshal(raw, &messages); err != nil {
		t.Fatalf("decode messages: %v", err)
	}
	if len(messages) != 2 || messages[0].Role != "system" || messages[0].Content != systemPrompt || messages[1].Role != "user" {
		t.Fatalf("messages = %#v", messages)
	}
	var payload decisionPayload
	if err := json.Unmarshal([]byte(messages[1].Content), &payload); err != nil {
		t.Fatalf("decode decision payload: %v", err)
	}
	if payload.Profile != profile.Profile {
		t.Errorf("profile input = %#v, want %#v", payload.Profile, profile.Profile)
	}
	for index, candidate := range payload.Candidates {
		if candidate.Index != index || candidate.Text != profile.Candidates[index] {
			t.Errorf("candidate %d = %#v", index, candidate)
		}
		if err := replies.ValidateModelCandidate(candidate.Text); err != nil {
			t.Errorf("candidate %d is unsafe: %v", index, err)
		}
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal([]byte(messages[1].Content), &fields); err != nil {
		t.Fatalf("decode payload fields: %v", err)
	}
	assertExactKeys(t, fields, "profile", "candidates")
	var profileFields map[string]json.RawMessage
	if err := json.Unmarshal(fields["profile"], &profileFields); err != nil {
		t.Fatalf("decode profile fields: %v", err)
	}
	assertExactKeys(t, profileFields,
		"version", "trigger", "category", "form", "structure", "length_bucket",
		"punctuation", "tone", "humor", "directness", "casing", "profanity_strength", "spice",
	)
	var candidates []map[string]json.RawMessage
	if err := json.Unmarshal(fields["candidates"], &candidates); err != nil {
		t.Fatalf("decode candidates: %v", err)
	}
	if len(candidates) != CandidateCount {
		t.Fatalf("candidate count = %d, want %d", len(candidates), CandidateCount)
	}
	for _, candidate := range candidates {
		assertExactKeys(t, candidate, "index", "text")
	}
}

func assertNoCanaries(t *testing.T, location string, value string) {
	t.Helper()
	lower := strings.ToLower(value)
	for _, forbidden := range []string{"jan", "kowalski", "captain", "123456789", "private.example", "report.pdf", "discord.gg", "discord.com/invite", "discord", "bosun", "smykland"} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("request %s contains canary %q", location, forbidden)
		}
	}
}

func TestSystemPromptKeepsSafetyAndDataBoundaries(t *testing.T) {
	t.Parallel()

	for _, rule := range []string{
		"Wybierz indeks jednej z ośmiu gotowych odpowiedzi",
		"wyłącznie do stałych etykiet profilu",
		"Nie zmieniaj tekstu odpowiedzi",
		"wymagany obiekt JSON",
	} {
		if !strings.Contains(systemPrompt, rule) {
			t.Fatalf("system prompt does not contain rule %q", rule)
		}
	}
	for _, forbidden := range []string{"discord", "bosun", "smykland"} {
		if strings.Contains(strings.ToLower(systemPrompt), forbidden) {
			t.Fatalf("system prompt contains forbidden term %q", forbidden)
		}
	}
}

func TestNewRejectsEveryModelOutsideAllowlist(t *testing.T) {
	t.Parallel()

	for _, model := range []string{"", "example/model", "private-person/private-model", "example/discord-model", "smykland/model"} {
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
				_, _ = io.WriteString(writer, decisionResponse(test.content))
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
				_, _ = io.WriteString(writer, decisionResponse(`{"choice":`+string(rune('0'+choice))+`}`))
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
		_, _ = io.WriteString(writer, `{"choices":[{"message":{"content":"częściowa odpowiedź"},"finish_reason":"stop","error":{"code":500}}]}`)
	}))
	defer server.Close()
	client := newTestClient(t, server.URL, time.Now)

	if choice, err := client.Complete(context.Background(), validInput()); choice != 0 || SafeFaultClass(err) != FaultRemote {
		t.Fatalf("Complete() = %d, %q; want remote fault", choice, SafeFaultClass(err))
	}
}

func TestCompleteAcceptsValidDecisionForAnyFinishReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		reason any
		omit   bool
	}{
		{name: "missing", omit: true},
		{name: "null", reason: nil},
		{name: "length", reason: "length"},
		{name: "error", reason: "error"},
		{name: "tool", reason: "tool"},
		{name: "tool calls", reason: "tool_calls"},
		{name: "other", reason: "content_filter"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			choice := map[string]any{"message": map[string]any{"content": `{"choice":5}`}}
			if !test.omit {
				choice["finish_reason"] = test.reason
			}
			encoded, _ := json.Marshal(map[string]any{"choices": []any{choice}})
			server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
				_, _ = writer.Write(encoded)
			}))
			defer server.Close()
			client := newTestClient(t, server.URL, time.Now)

			selected, err := client.Complete(context.Background(), validInput())
			if selected != 5 || err != nil {
				t.Fatalf("Complete() = %d, %v; want choice 5", selected, err)
			}
		})
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
				APIKey: "test-key", Model: ModelGeminiFlashLite, BalanceCooldown: 15 * time.Minute, RequestTimeout: 5 * time.Second,
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
		APIKey: "test-key", Model: ModelGeminiFlashLite, BalanceCooldown: 15 * time.Minute, RequestTimeout: 5 * time.Second,
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
