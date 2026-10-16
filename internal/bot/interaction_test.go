package bot

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

func TestAskAcknowledgesThenEditsDeferredResponse(t *testing.T) {
	t.Parallel()

	service := newInteractionTestService(t, 1, 1, nil)
	acknowledged := false
	event := testAskEventWithResponder(t, func(responseType discord.InteractionResponseType, data discord.InteractionResponseData, _ ...rest.RequestOpt) error {
		if responseType != discord.InteractionResponseTypeDeferredCreateMessage || data != nil {
			return errors.New("unexpected acknowledgement")
		}
		acknowledged = true
		return nil
	})
	edits := make(chan recordedInteractionEdit, 1)
	editor := interactionEditorFunc(func(ctx context.Context, applicationID snowflake.ID, token string, update discord.MessageUpdate) error {
		edits <- recordedInteractionEdit{ctx: ctx, applicationID: applicationID, token: token, update: update}
		return nil
	})

	service.handleApplicationCommand(event, editor)
	if !acknowledged {
		t.Fatal("the interaction was not acknowledged before queue submission")
	}
	select {
	case edit := <-edits:
		if edit.applicationID != 99 || edit.token != "interaction-token" {
			t.Fatalf("interaction target = %d, %q", edit.applicationID, edit.token)
		}
		if edit.ctx.Err() != nil {
			t.Fatalf("interaction edit context error = %v", edit.ctx.Err())
		}
		assertInteractionUpdate(t, edit.update, "private reply text")
	case <-time.After(time.Second):
		t.Fatal("the deferred interaction response was not edited")
	}
}

func TestAskAcknowledgesWhileAllInteractionWorkersAreBlocked(t *testing.T) {
	t.Parallel()

	service := newInteractionTestService(t, 2, 2, nil)
	blockExecutorWorkers(t, service.interactions, 2)
	acknowledged := make(chan struct{}, 1)
	event := testAskEventWithResponder(t, func(responseType discord.InteractionResponseType, _ discord.InteractionResponseData, _ ...rest.RequestOpt) error {
		if responseType != discord.InteractionResponseTypeDeferredCreateMessage {
			return errors.New("unexpected acknowledgement type")
		}
		acknowledged <- struct{}{}
		return nil
	})
	editorCalled := make(chan struct{}, 1)
	done := make(chan struct{})
	go func() {
		service.handleApplicationCommand(event, interactionEditorFunc(func(context.Context, snowflake.ID, string, discord.MessageUpdate) error {
			editorCalled <- struct{}{}
			return nil
		}))
		close(done)
	}()

	select {
	case <-acknowledged:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("blocked interaction workers delayed the acknowledgement")
	}
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("the event path did not return after queue submission")
	}
	select {
	case <-editorCalled:
		t.Fatal("the queued edit started while all workers were blocked")
	default:
	}
}

func TestAskOverloadEditsDeferredResponseWithSafeMessage(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&output, nil))
	service := newInteractionTestService(t, 1, 1, logger)
	blockExecutorWorkers(t, service.interactions, 1)
	if !service.interactions.Submit(func(ctx context.Context) { <-ctx.Done() }) {
		t.Fatal("the interaction queue rejected the setup job")
	}
	acknowledged := false
	event := testAskEventWithResponder(t, func(responseType discord.InteractionResponseType, _ discord.InteractionResponseData, _ ...rest.RequestOpt) error {
		acknowledged = responseType == discord.InteractionResponseTypeDeferredCreateMessage
		return nil
	})
	var overloadUpdate discord.MessageUpdate
	var overloadDeadline time.Time
	service.handleApplicationCommand(event, interactionEditorFunc(func(ctx context.Context, _ snowflake.ID, _ string, update discord.MessageUpdate) error {
		overloadUpdate = update
		overloadDeadline, _ = ctx.Deadline()
		return nil
	}))

	if !acknowledged {
		t.Fatal("the overloaded interaction was not acknowledged")
	}
	assertInteractionUpdate(t, overloadUpdate, interactionOverloadMessage)
	remaining := time.Until(overloadDeadline)
	if overloadDeadline.IsZero() || remaining <= 0 || remaining > interactionBusyTimeout {
		t.Fatalf("overload edit deadline remaining = %v", remaining)
	}
	logOutput := output.String()
	if !strings.Contains(logOutput, "interaction_queue_full") {
		t.Fatalf("overload log = %q, want queue reason", logOutput)
	}
	if strings.Contains(logOutput, "co teraz?") || strings.Contains(logOutput, "private reply text") {
		t.Fatalf("overload log contains private content: %s", logOutput)
	}
}

func TestAskAcknowledgementFailureStopsBeforeEdit(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	service := newInteractionTestService(t, 1, 1, slog.New(slog.NewJSONHandler(&output, nil)))
	event := testAskEventWithResponder(t, func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error {
		return errors.New("private acknowledgement error")
	})
	editorCalled := false
	service.handleApplicationCommand(event, interactionEditorFunc(func(context.Context, snowflake.ID, string, discord.MessageUpdate) error {
		editorCalled = true
		return nil
	}))

	if editorCalled {
		t.Fatal("the interaction edit ran after acknowledgement failure")
	}
	logOutput := output.String()
	if !strings.Contains(logOutput, "discord interaction acknowledgement failed") {
		t.Fatalf("acknowledgement log = %q, want failure event", logOutput)
	}
	for _, privateText := range []string{"private acknowledgement error", "co teraz?", "private reply text"} {
		if strings.Contains(logOutput, privateText) {
			t.Fatalf("acknowledgement log contains private content: %s", logOutput)
		}
	}
}

func TestServiceCloseCancelsInteractionEdit(t *testing.T) {
	t.Parallel()

	service := newInteractionTestService(t, 1, 1, nil)
	started := make(chan struct{})
	canceled := make(chan struct{})
	event := testAskEventWithResponder(t, func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error {
		return nil
	})
	service.handleApplicationCommand(event, interactionEditorFunc(func(ctx context.Context, _ snowflake.ID, _ string, _ discord.MessageUpdate) error {
		close(started)
		<-ctx.Done()
		close(canceled)
		return ctx.Err()
	}))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the interaction edit did not start")
	}

	closeService(t, service)
	select {
	case <-canceled:
	default:
		t.Fatal("service shutdown did not cancel the interaction edit")
	}
}

type interactionEditorFunc func(context.Context, snowflake.ID, string, discord.MessageUpdate) error

func (f interactionEditorFunc) Update(ctx context.Context, applicationID snowflake.ID, token string, update discord.MessageUpdate) error {
	return f(ctx, applicationID, token, update)
}

type recordedInteractionEdit struct {
	ctx           context.Context
	applicationID snowflake.ID
	token         string
	update        discord.MessageUpdate
}

func newInteractionTestService(t *testing.T, workers int, capacity int, logger *slog.Logger) *Service {
	t.Helper()
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	service, err := newService(42, 99, "Bosun", fixedSelector{}, logger, mustExecutor(t, 1, 1), mustExecutor(t, workers, capacity))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { closeService(t, service) })
	return service
}

func blockExecutorWorkers(t *testing.T, executor *executor, workers int) {
	t.Helper()
	started := make(chan struct{}, workers)
	for range workers {
		if !executor.Submit(func(ctx context.Context) {
			started <- struct{}{}
			<-ctx.Done()
		}) {
			t.Fatal("the executor rejected a worker blocker")
		}
	}
	for range workers {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("an executor worker did not start")
		}
	}
}

func assertInteractionUpdate(t *testing.T, update discord.MessageUpdate, content string) {
	t.Helper()
	if update.Content == nil || *update.Content != content {
		t.Fatalf("interaction content = %v, want %q", update.Content, content)
	}
	assertAllowedMentionsDisabled(t, update.AllowedMentions)
}
