package bot

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/smykla-skalski/bosun/internal/openrouter"
	"github.com/smykla-skalski/bosun/internal/replies"
)

func TestResolveReplyUsesAnonymousMessageProfile(t *testing.T) {
	t.Parallel()

	var captured openrouter.Input
	model := replyModelFunc{
		complete: func(_ context.Context, input openrouter.Input) (int, error) {
			captured = input
			return 3, nil
		},
	}
	service := &Service{model: model, botHandle: "Bosun"}
	catalog, err := replies.LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}
	selector := replies.NewSelector(catalog, replies.WithIndex(func(int) int { return 0 }))
	batch, err := selector.SelectBatch("private-user-id", replies.CategoryRequest)
	if err != nil {
		t.Fatalf("SelectBatch() error = %v", err)
	}
	t.Cleanup(func() { _ = selector.CancelBatch(batch) })
	selections := batch.Selections
	canary := "Jan Kowalski <@99> https://private.example/secret DiScOrD.Gg/private-invite DiScOrD.CoM/InViTe/private-invite private-plan.pdf Discord Bosun smykland, zrób to?!"
	result := service.resolveReply(context.Background(), canary, TriggerMention, replies.Category("request"), selections)
	if result.text != selections[3].Reply.Text || result.source != replySourceOpenRouter || result.faultClass != noFault {
		t.Fatalf("resolveReply() result = %#v", result)
	}
	if captured.Profile.Trigger != "mention" || captured.Profile.Category != "request" || captured.Profile.Spice != "harsh" {
		t.Fatalf("model profile = %#v", captured)
	}
	if captured.CandidateType != "request" || captured.Profile.ResponseMode != "task_roast" {
		t.Fatalf("model reply type = %#v", captured)
	}
	for index, candidate := range captured.Candidates {
		if candidate != selections[index].Reply.Text {
			t.Fatalf("candidate %d = %q", index, candidate)
		}
		if err := replies.ValidateModelCandidate(candidate); err != nil {
			t.Fatalf("candidate %d is unsafe: %v", index, err)
		}
	}
	encoded, err := json.Marshal(captured)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	lower := strings.ToLower(string(encoded))
	for _, forbidden := range []string{"jan", "kowalski", "99", "private-user-id", "private.example", "private-plan.pdf", "discord", "bosun", "smykland"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("model profile contains canary %q", forbidden)
		}
	}
}

func TestServiceKeepsNegatedJokeProfileGeneric(t *testing.T) {
	t.Parallel()

	profiles := make(chan openrouter.Input, 1)
	model := replyModelFunc{complete: func(_ context.Context, input openrouter.Input) (int, error) {
		profiles <- input
		return 0, nil
	}}
	service, err := NewService(
		42, 99, "Bosun", fixedSelector{}, slog.New(slog.NewTextHandler(io.Discard, nil)), WithReplyModel(model),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { closeService(t, service) })
	guildID := snowflake.ID(42)
	service.handleMessage(&guildID, 2, discord.Message{
		ID: 1, GuildID: &guildID, ChannelID: 2, Author: discord.User{ID: 7}, Content: "Bosun, nie żartuję.",
	}, &recordingSender{})

	select {
	case input := <-profiles:
		profile := input.Profile
		if profile.Category != "generic" || profile.Tone != "neutral" || profile.Humor != "none" {
			t.Fatalf("negated joke profile = %#v", profile)
		}
	case <-time.After(time.Second):
		t.Fatal("the model did not receive a profile")
	}
}

func TestResolveReplyUsesStaticFallbackForModelFault(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		modelChoice int
		modelErr    error
		wantFault   string
	}{
		{name: "timeout", modelErr: context.DeadlineExceeded, wantFault: openrouter.FaultTimeout},
		{name: "invalid completion", modelErr: classifiedTestFault(openrouter.FaultInvalidOutput), wantFault: openrouter.FaultInvalidOutput},
		{name: "out of range", modelChoice: openrouter.CandidateCount, wantFault: openrouter.FaultInvalidOutput},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := &Service{model: replyModelFunc{
				complete: func(context.Context, openrouter.Input) (int, error) {
					return test.modelChoice, test.modelErr
				},
			}}
			result := service.resolveReply(context.Background(), "pytanie", TriggerAsk, replies.Category("question"), fixedSelections())
			if result.text != "private reply text" || result.source != replySourceStatic || result.faultClass != test.wantFault {
				t.Fatalf("resolveReply() fallback = %#v", result)
			}
		})
	}
}

func TestResolveReplyDoesNotSendConfiguredHandleInCandidate(t *testing.T) {
	t.Parallel()

	var modelCalls atomic.Int32
	service := &Service{botHandle: "Kapitan", model: replyModelFunc{
		complete: func(context.Context, openrouter.Input) (int, error) {
			modelCalls.Add(1)
			return 4, nil
		},
	}}
	selections := fixedSelections()
	selections[4].Reply.Text = "Mniej dymu, więcej współrzędnych, kapitanie."

	result := service.resolveReply(
		context.Background(), "Kapitan, odpowiedz.", TriggerHandle, replies.CategoryRequest, selections,
	)
	if result.text != selections[0].Reply.Text || result.source != replySourceStatic {
		t.Fatalf("resolveReply() fallback = %#v", result)
	}
	if result.faultClass != openrouter.FaultInvalidInput {
		t.Fatalf("resolveReply() fault = %q, want %q", result.faultClass, openrouter.FaultInvalidInput)
	}
	if modelCalls.Load() != 0 {
		t.Fatalf("model calls = %d, want 0", modelCalls.Load())
	}
}

func TestResolveReplyUsesStaticFallbackForRejectedModelOutput(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(writer, `{"choices":[{"message":{"content":"Jutro cię zabiję."},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	client, err := openrouter.New(openrouter.Config{
		APIKey: "test-key", Model: openrouter.ModelJev,
		BalanceCooldown: time.Minute, RequestTimeout: time.Second,
	}, openrouter.WithBaseURL(server.URL))
	if err != nil {
		t.Fatalf("openrouter.New() error = %v", err)
	}
	service := &Service{model: client, botHandle: "Bosun"}

	result := service.resolveReply(
		context.Background(), "zrób coś", TriggerAsk, replies.Category("request"), fixedSelections(),
	)
	if result.text != "private reply text" || result.source != replySourceStatic {
		t.Fatalf("resolveReply() fallback = %#v", result)
	}
	if result.faultClass != openrouter.FaultInvalidOutput {
		t.Fatalf("resolveReply() fault = %q", result.faultClass)
	}
}

func TestResolveReplyPassesCancellationToModel(t *testing.T) {
	t.Parallel()

	started := make(chan struct{})
	service := &Service{model: replyModelFunc{
		complete: func(ctx context.Context, _ openrouter.Input) (int, error) {
			close(started)
			<-ctx.Done()
			return 0, ctx.Err()
		},
	}}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan replyResult, 1)
	go func() {
		result <- service.resolveReply(ctx, "pytanie", TriggerAsk, replies.Category("question"), fixedSelections())
	}()
	<-started
	cancel()

	select {
	case got := <-result:
		if got.text != "private reply text" || got.faultClass != openrouter.FaultCanceled {
			t.Fatalf("resolveReply() canceled result = %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatal("resolveReply() did not stop after cancellation")
	}
}

func TestAskDefersBeforeModelWork(t *testing.T) {
	t.Parallel()

	var acknowledged atomic.Bool
	modelCalled := make(chan struct{}, 1)
	model := replyModelFunc{
		complete: func(context.Context, openrouter.Input) (int, error) {
			if !acknowledged.Load() {
				return 0, errors.New("model started before acknowledgement")
			}
			modelCalled <- struct{}{}
			return 0, nil
		},
	}
	service := newInteractionTestService(t, 1, 1, slog.New(slog.NewTextHandler(io.Discard, nil)), WithReplyModel(model))
	event := testAskEventWithResponder(t, func(responseType discord.InteractionResponseType, _ discord.InteractionResponseData, _ ...rest.RequestOpt) error {
		if responseType != discord.InteractionResponseTypeDeferredCreateMessage {
			return errors.New("unexpected acknowledgement type")
		}
		acknowledged.Store(true)
		return nil
	})
	edited := make(chan struct{}, 1)
	service.handleApplicationCommand(event, interactionEditorFunc(func(context.Context, snowflake.ID, string, discord.MessageUpdate) error {
		edited <- struct{}{}
		return nil
	}))

	select {
	case <-modelCalled:
	case <-time.After(time.Second):
		t.Fatal("model work did not start")
	}
	select {
	case <-edited:
	case <-time.After(time.Second):
		t.Fatal("the deferred reply was not edited")
	}
}

type replyModelFunc struct {
	complete func(context.Context, openrouter.Input) (int, error)
}

func (m replyModelFunc) Complete(ctx context.Context, input openrouter.Input) (int, error) {
	return m.complete(ctx, input)
}

type classifiedTestFault string

func (f classifiedTestFault) Error() string {
	return string(f)
}

func (f classifiedTestFault) FaultClass() string {
	return string(f)
}
