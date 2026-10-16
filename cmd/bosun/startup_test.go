package main

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

func TestRegisterGuildCommandsUsesRunContext(t *testing.T) {
	t.Parallel()

	requestStarted := make(chan struct{}, 1)
	httpClient := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		requestStarted <- struct{}{}
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	restClient := rest.NewClient("token", rest.WithHTTPClient(httpClient), rest.WithURL("https://discord.invalid"))
	applications := rest.NewApplications(restClient)

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		result <- registerGuildCommands(ctx, applications, snowflake.ID(1), snowflake.ID(2))
	}()

	select {
	case <-requestStarted:
	case <-time.After(time.Second):
		t.Fatal("guild command request did not start")
	}
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("registerGuildCommands() error = %v, want context cancellation", err)
		}
	case <-time.After(time.Second):
		t.Fatal("guild command request did not stop after context cancellation")
	}

	closeCtx, closeCancel := context.WithTimeout(context.Background(), time.Second)
	defer closeCancel()
	restClient.Close(closeCtx)
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}
