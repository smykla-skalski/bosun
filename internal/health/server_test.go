package health

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
)

func TestHealthEndpointsTrackReadiness(t *testing.T) {
	t.Parallel()

	var ready atomic.Bool
	server, err := New(":8080", ready.Load)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	assertStatus(t, server.Handler(), "/livez", http.StatusOK)
	assertStatus(t, server.Handler(), "/readyz", http.StatusServiceUnavailable)
	ready.Store(true)
	assertStatus(t, server.Handler(), "/readyz", http.StatusOK)
	ready.Store(false)
	assertStatus(t, server.Handler(), "/readyz", http.StatusServiceUnavailable)
}

func TestHealthServerRejectsInvalidInputs(t *testing.T) {
	t.Parallel()

	if _, err := New("", func() bool { return true }); err == nil {
		t.Fatal("New() accepted an empty address")
	}
	if _, err := New(":8080", nil); err == nil {
		t.Fatal("New() accepted a nil readiness function")
	}
}

func assertStatus(t *testing.T, handler http.Handler, path string, want int) {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, path, nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != want {
		t.Fatalf("GET %s status = %d, want %d", path, recorder.Code, want)
	}
}
