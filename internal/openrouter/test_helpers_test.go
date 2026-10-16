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
		Model:           ModelGeminiFlashLite,
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
			Structure: "simple", LengthBucket: "short", Punctuation: "question", Tone: "neutral",
			Humor: "none", Directness: "direct", Casing: "lower", ProfanityStrength: "none", Spice: "sharp",
		},
		Candidates: [CandidateCount]string{
			"Twój pomysł wszedł na pokład bez mapy i kompasu. Spróbuj ponownie, tym razem z choćby szkicem celu.",
			"Twój pomysł wszedł na pokład bez mapy i kompasu. Dodaj konkret, zanim reszta zdania zacznie panikować.",
			"Twój pomysł wszedł na pokład bez mapy i kompasu. Nie naprawię kursu, którego nikt nie wyznaczył.",
			"Twój pomysł wszedł na pokład bez mapy i kompasu. Następna wersja niech zawiera także powód.",
			"Twój pomysł wszedł na pokład bez mapy i kompasu. Zbierz fakty, zanim znowu wypuścisz ten balon.",
			"Twój pomysł wszedł na pokład bez mapy i kompasu. Daj temu minutę myślenia i drugi przecinek.",
			"Twój pomysł wszedł na pokład bez mapy i kompasu. Wróć z pytaniem, które wie, czego chce.",
			"Twój pomysł wszedł na pokład bez mapy i kompasu. Nawet dobra mina nie zastąpi tu danych.",
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
	return decisionResponse(`{"choice":1}`)
}

func decisionResponse(content string) string {
	encoded, _ := json.Marshal(map[string]any{
		"choices": []any{map[string]any{
			"message": map[string]any{"content": content}, "finish_reason": "stop",
		}},
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
