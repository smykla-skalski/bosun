package health

import (
	"context"
	"errors"
	"net/http"
	"time"
)

type ReadyFunc func() bool

type Server struct {
	httpServer *http.Server
}

func New(address string, ready ReadyFunc) (*Server, error) {
	if address == "" {
		return nil, errors.New("health address is empty")
	}
	if ready == nil {
		return nil, errors.New("health readiness function is nil")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /livez", func(writer http.ResponseWriter, _ *http.Request) {
		writeStatus(writer, http.StatusOK, "ok")
	})
	mux.HandleFunc("GET /readyz", func(writer http.ResponseWriter, _ *http.Request) {
		if !ready() {
			writeStatus(writer, http.StatusServiceUnavailable, "not ready")
			return
		}
		writeStatus(writer, http.StatusOK, "ok")
	})

	return &Server{
		httpServer: &http.Server{
			Addr:              address,
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}, nil
}

func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func writeStatus(writer http.ResponseWriter, status int, body string) {
	writer.Header().Set("Content-Type", "text/plain; charset=utf-8")
	writer.WriteHeader(status)
	_, _ = writer.Write([]byte(body + "\n"))
}
