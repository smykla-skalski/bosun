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

	"github.com/smykla-skalski/bosun/internal/replies"
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
		deadline, _ := ctx.Deadline()
		edits <- recordedInteractionEdit{
			initialErr: ctx.Err(), deadline: deadline,
			applicationID: applicationID, token: token, update: update,
		}
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
		if edit.initialErr != nil || edit.deadline.IsZero() {
			t.Fatalf("interaction edit context = error %v, deadline %v", edit.initialErr, edit.deadline)
		}
		assertInteractionUpdate(t, edit.update, "private reply text")
	case <-time.After(time.Second):
		t.Fatal("the deferred interaction response was not edited")
	}
}

func TestAskFinalEditCanTakeMoreThanHalfASecond(t *testing.T) {
	t.Parallel()

	service := newInteractionTestService(t, 1, 1, nil)
	event := successfulAskEvent(t)
	result := make(chan error, 1)
	service.handleApplicationCommand(event, interactionEditorFunc(func(ctx context.Context, _ snowflake.ID, _ string, _ discord.MessageUpdate) error {
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) < 4*time.Second {
			result <- errors.New("the final edit does not have the production deadline")
			return nil
		}
		timer := time.NewTimer(600 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-timer.C:
			result <- nil
			return nil
		case <-ctx.Done():
			result <- ctx.Err()
			return ctx.Err()
		}
	}))

	select {
	case err := <-result:
		if err != nil {
			t.Fatalf("delayed final edit failed: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("the delayed final edit did not finish")
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
	assertInteractionUpdate(t, overloadUpdate, "private reply text")
	remaining := time.Until(overloadDeadline)
	if overloadDeadline.IsZero() || remaining <= 0 || remaining > interactionFinalTimeout {
		t.Fatalf("overload edit deadline remaining = %v", remaining)
	}
	logOutput := output.String()
	if !strings.Contains(logOutput, `"fault_class":"local_overload"`) {
		t.Fatalf("overload log = %q, want safe fault class", logOutput)
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
	if !strings.Contains(logOutput, `"fault_class":"discord_ack"`) {
		t.Fatalf("acknowledgement log = %q, want failure event", logOutput)
	}
	for _, privateText := range []string{"private acknowledgement error", "co teraz?", "private reply text"} {
		if strings.Contains(logOutput, privateText) {
			t.Fatalf("acknowledgement log contains private content: %s", logOutput)
		}
	}
}

func TestAskSelectionCapacityFaultFinalizesDeferredResponse(t *testing.T) {
	selector := selectorFunc{selectBatch: func(string, replies.Category) (replies.Batch, error) {
		return replies.Batch{}, replies.ErrReservationCapacity
	}}
	service := newInteractionServiceWithSelector(t, selector, mustExecutor(t, 1, 1))
	edits := make(chan discord.MessageUpdate, 2)
	service.handleApplicationCommand(successfulAskEvent(t), interactionEditorFunc(func(
		_ context.Context,
		_ snowflake.ID,
		_ string,
		update discord.MessageUpdate,
	) error {
		edits <- update
		return nil
	}))
	assertOneFailureUpdate(t, edits)
}

func TestAskRejectedQueueSelectionFaultFinalizesDeferredResponse(t *testing.T) {
	selector := selectorFunc{selectBatch: func(string, replies.Category) (replies.Batch, error) {
		return replies.Batch{}, replies.ErrReservationCapacity
	}}
	executor := mustExecutor(t, 1, 1)
	blockExecutorWorkers(t, executor, 1)
	if !executor.Submit(func(ctx context.Context) { <-ctx.Done() }) {
		t.Fatal("the interaction queue rejected the setup job")
	}
	service := newInteractionServiceWithSelector(t, selector, executor)
	edits := make(chan discord.MessageUpdate, 2)
	service.handleApplicationCommand(successfulAskEvent(t), interactionEditorFunc(func(
		_ context.Context,
		_ snowflake.ID,
		_ string,
		update discord.MessageUpdate,
	) error {
		edits <- update
		return nil
	}))
	assertOneFailureUpdate(t, edits)
}

func TestServiceCloseWaitsForRejectedFinalEditsTogether(t *testing.T) {
	t.Parallel()

	const editCount = 3
	service := newInteractionTestService(t, 1, 1, nil)
	blockExecutorWorkers(t, service.interactions, 1)
	if !service.interactions.Submit(func(ctx context.Context) { <-ctx.Done() }) {
		t.Fatal("the interaction queue rejected the setup job")
	}

	started := make(chan struct{}, editCount+1)
	release := make(chan struct{})
	handled := make(chan struct{}, editCount)
	editor := interactionEditorFunc(func(ctx context.Context, _ snowflake.ID, _ string, _ discord.MessageUpdate) error {
		started <- struct{}{}
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	for range editCount {
		event := successfulAskEvent(t)
		go func() {
			service.handleApplicationCommand(event, editor)
			handled <- struct{}{}
		}()
	}
	for range editCount {
		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("the rejected final edits did not start together")
		}
	}

	closeCtx, cancelClose := context.WithTimeout(context.Background(), time.Second)
	defer cancelClose()
	closed := make(chan error, 1)
	go func() { closed <- service.Close(closeCtx) }()
	select {
	case <-service.interactions.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("service shutdown did not start")
	}
	select {
	case err := <-closed:
		t.Fatalf("Service.Close() returned before final edits finished: %v", err)
	default:
	}

	close(release)
	for range editCount {
		select {
		case <-handled:
		case <-time.After(time.Second):
			t.Fatal("a rejected interaction did not finish")
		}
	}
	if err := <-closed; err != nil {
		t.Fatalf("Service.Close() error = %v", err)
	}
	select {
	case <-started:
		t.Fatal("a rejected interaction received multiple final updates")
	default:
	}
}

func TestServiceCloseWaitsForBoundedInteractionEdit(t *testing.T) {
	t.Parallel()

	const testFinalTimeout = 40 * time.Millisecond
	service := newInteractionTestService(t, 1, 1, nil, func(service *Service) {
		service.finalTimeout = testFinalTimeout
	})
	started := make(chan struct{})
	finished := make(chan error, 1)
	event := testAskEventWithResponder(t, func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error {
		return nil
	})
	service.handleApplicationCommand(event, interactionEditorFunc(func(ctx context.Context, _ snowflake.ID, _ string, _ discord.MessageUpdate) error {
		close(started)
		<-ctx.Done()
		finished <- ctx.Err()
		return ctx.Err()
	}))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("the interaction edit did not start")
	}

	startedClose := time.Now()
	closeService(t, service)
	if elapsed := time.Since(startedClose); elapsed > 500*time.Millisecond {
		t.Fatalf("service shutdown elapsed = %v, want a bounded wait", elapsed)
	}
	select {
	case err := <-finished:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("final edit context error = %v, want deadline", err)
		}
	default:
		t.Fatal("service shutdown returned before the bounded edit finished")
	}
}

type interactionEditorFunc func(context.Context, snowflake.ID, string, discord.MessageUpdate) error

func (f interactionEditorFunc) Update(ctx context.Context, applicationID snowflake.ID, token string, update discord.MessageUpdate) error {
	return f(ctx, applicationID, token, update)
}

type recordedInteractionEdit struct {
	initialErr    error
	deadline      time.Time
	applicationID snowflake.ID
	token         string
	update        discord.MessageUpdate
}

func newInteractionTestService(t *testing.T, workers int, capacity int, logger *slog.Logger, options ...ServiceOption) *Service {
	t.Helper()
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	service, err := newService(42, 99, "Bosun", fixedSelector{}, logger, mustExecutor(t, 1, 1), mustExecutor(t, workers, capacity), options...)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { closeService(t, service) })
	return service
}

func newInteractionServiceWithSelector(t *testing.T, selector ReplySelector, interactions *executor) *Service {
	t.Helper()
	service, err := newService(
		42, 99, "Bosun", selector, slog.New(slog.NewTextHandler(io.Discard, nil)),
		mustExecutor(t, 1, 1), interactions,
	)
	if err != nil {
		t.Fatalf("newService() error = %v", err)
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

func assertOneFailureUpdate(t *testing.T, edits <-chan discord.MessageUpdate) {
	t.Helper()
	select {
	case update := <-edits:
		assertInteractionUpdate(t, update, interactionFailureMessage)
	case <-time.After(time.Second):
		t.Fatal("the deferred interaction response was not finalized")
	}
	select {
	case <-edits:
		t.Fatal("the deferred interaction response received multiple final updates")
	case <-time.After(20 * time.Millisecond):
	}
}
