package openrouter

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/smykla-skalski/bosun/internal/messageprofile"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type errorReadCloser struct{}

func (errorReadCloser) Read([]byte) (int, error) {
	return 0, errors.New("test read fault")
}

func (errorReadCloser) Close() error {
	return nil
}

func testResponse(status int, body string, retryAfter string) *http.Response {
	response := &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(body)),
	}
	if retryAfter != "" {
		response.Header.Set("Retry-After", retryAfter)
	}
	return response
}

func BenchmarkOpenCreditCircuit(b *testing.B) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.WriteHeader(http.StatusPaymentRequired)
		_, _ = io.WriteString(writer, `{"error":{"code":402}}`)
	}))
	defer server.Close()
	client := newTestClient(b, server.URL, time.Now)
	_, _ = client.Complete(context.Background(), validInput())

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		_, _ = client.Complete(context.Background(), validInput())
	}
}

type testHelper interface {
	Helper()
	Fatalf(string, ...any)
}

func newTestClient(t testHelper, baseURL string, now func() time.Time) *Client {
	t.Helper()
	client, err := New(Config{
		APIKey:          "test-key",
		Model:           ModelJev,
		BalanceCooldown: 15 * time.Minute,
		RequestTimeout:  5 * time.Second,
	}, WithBaseURL(baseURL), WithClock(now))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	return client
}

func validInput() Input {
	return Input{
		Profile: messageprofile.Profile{
			Version: messageprofile.Version, Trigger: "ask", Category: "question", Form: "question",
			ConversationRole: "explicit_request", QuestionKind: "information", ResponseMode: "answer_then_roast",
			Structure: "simple", LengthBucket: "short", Punctuation: "question", Tone: "neutral",
			Humor: "none", Directness: "direct", Casing: "lower", ProfanityStrength: "none", Spice: "sharp",
		},
		CandidateType: "question",
		Candidates: [CandidateCount]string{
			"Twój pomysł wszedł na pokład bez mapy i kompasu, a rozsądek ogląda rejs z łodzi ratunkowej",
			"To pytanie ma więcej pewności niż treści, bo znak zapytania robi całą robotę",
			"Widzę odwagę, lecz sens został w porcie, więc logika wysłała za tobą list gończy",
			"Ten plan brzmi jak szkic napisany podczas sztormu, a fakty wystąpiły o zakaz zbliżania",
			"Pytasz śmiało, jakby szczegóły były opcjonalne, choć nawet chaos poprosił o czytelniejszy plan",
			"To zdanie dopłynęło, ale argument wypadł za burtę, więc odpowiedź musi posprzątać po twojej myśli",
			"Masz kierunek, tylko każdy drogowskaz wskazuje gdzie indziej, bo przypadek wniósłby więcej precyzji",
			"Pomysł jest świeży, bo rozsądek jeszcze go nie dotknął, a puenta sama poprosiła o lepszego autora",
		},
	}
}

func assertExactKeys(t *testing.T, fields map[string]json.RawMessage, keys ...string) {
	t.Helper()
	if len(fields) != len(keys) {
		t.Fatalf("field count = %d, want %d", len(fields), len(keys))
	}
	for _, key := range keys {
		if _, ok := fields[key]; !ok {
			t.Errorf("request lacks %q", key)
		}
	}
}

func completionJSON(content string) string {
	_ = content
	return decisionJSON(1)
}

func rawDecisionResponse(content string) string {
	return `{"answers":{"reply":` + content + `}}`
}

func decisionJSON(choice int) string {
	probabilities := make(map[string]float64, CandidateCount)
	for _, candidateID := range candidateIDs {
		probabilities[candidateID] = 0
	}
	probabilities[candidateIDs[choice]] = 1
	return decisionJSONWithProbabilities(choice, probabilities, 1)
}

func decisionJSONWithProbabilities(choice int, probabilities map[string]float64, confidence float64) string {
	encoded, _ := json.Marshal(map[string]any{
		"model": ModelJev,
		"answers": map[string]any{
			"reply": map[string]any{
				"type": "choice", "choice": candidateIDs[choice],
				"probabilities": probabilities, "confidence": confidence,
			},
		},
	})
	return string(encoded)
}

func assertRequestField[T comparable](t *testing.T, body map[string]json.RawMessage, name string, want T) {
	t.Helper()
	var got T
	if err := json.Unmarshal(body[name], &got); err != nil {
		t.Errorf("decode %s: %v", name, err)
		return
	}
	if got != want {
		t.Errorf("%s = %#v, want %#v", name, got, want)
	}
}

type testClock struct {
	nanoseconds atomic.Int64
}

func newTestClock(now time.Time) *testClock {
	clock := &testClock{}
	clock.nanoseconds.Store(now.UnixNano())
	return clock
}

func (c *testClock) Now() time.Time {
	return time.Unix(0, c.nanoseconds.Load())
}

func (c *testClock) Add(duration time.Duration) {
	c.nanoseconds.Add(int64(duration))
}
