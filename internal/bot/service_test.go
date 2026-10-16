package bot

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/smykla-skalski/bosun/internal/openrouter"
	"github.com/smykla-skalski/bosun/internal/replies"
)

func TestServiceAcceptsAskOnlyForConfiguredGuildAndHuman(t *testing.T) {
	t.Parallel()

	service := &Service{guildID: 42, botID: 99}
	guildID := snowflake.ID(42)
	otherGuildID := snowflake.ID(43)
	tests := []struct {
		name        string
		guildID     *snowflake.ID
		user        discord.User
		commandName string
		question    string
		hasQuestion bool
		want        bool
	}{
		{name: "valid", guildID: &guildID, user: discord.User{ID: 7}, commandName: "ask", question: "co teraz?", hasQuestion: true, want: true},
		{name: "direct message", user: discord.User{ID: 7}, commandName: "ask", question: "co teraz?", hasQuestion: true},
		{name: "other guild", guildID: &otherGuildID, user: discord.User{ID: 7}, commandName: "ask", question: "co teraz?", hasQuestion: true},
		{name: "other command", guildID: &guildID, user: discord.User{ID: 7}, commandName: "other", question: "co teraz?", hasQuestion: true},
		{name: "missing question", guildID: &guildID, user: discord.User{ID: 7}, commandName: "ask"},
		{name: "empty question", guildID: &guildID, user: discord.User{ID: 7}, commandName: "ask", question: "  ", hasQuestion: true},
		{name: "bot", guildID: &guildID, user: discord.User{ID: 7, Bot: true}, commandName: "ask", question: "co teraz?", hasQuestion: true},
		{name: "self", guildID: &guildID, user: discord.User{ID: 99}, commandName: "ask", question: "co teraz?", hasQuestion: true},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got := service.acceptsAsk(test.guildID, test.user, test.commandName, test.question, test.hasQuestion)
			if got != test.want {
				t.Fatalf("acceptsAsk() = %v, want %v", got, test.want)
			}
		})
	}
}

func TestServiceLogsOnlySafeReplyFields(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	service := &Service{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	selection := fixedSelection()
	service.logFault(selection, staticResult("private reply text", "timeout"))
	var record map[string]any
	if err := json.Unmarshal(output.Bytes(), &record); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	allowed := map[string]bool{
		"time": true, "level": true, "msg": true, "source": true, "tier": true,
		"static_reply_id": true, "fault_class": true,
	}
	for key := range record {
		if !allowed[key] {
			t.Fatalf("reply log contains unsafe field %q", key)
		}
	}
	for _, privateText := range []string{"private reply text", "private message text", "private/model"} {
		if strings.Contains(output.String(), privateText) {
			t.Fatalf("reply log contains private content: %s", output.String())
		}
	}
}

func TestServicePassesLocalMessageCategoryToSelector(t *testing.T) {
	t.Parallel()

	categories := make(chan replies.Category, 1)
	selector := selectorFunc{selectBatch: func(_ string, category replies.Category) (replies.Batch, error) {
		categories <- category
		return replies.Batch{Selections: fixedSelections()}, nil
	}}
	service, err := NewService(42, 99, "Bosun", selector, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { closeService(t, service) })
	guildID := snowflake.ID(42)
	service.handleMessage(&guildID, 2, discord.Message{
		ID: 1, GuildID: &guildID, ChannelID: 2, Author: discord.User{ID: 7}, Content: "Bosun, zrób to teraz",
	}, &recordingSender{})
	select {
	case category := <-categories:
		if category != replies.CategoryRequest {
			t.Fatalf("selector category = %q, want %q", category, replies.CategoryRequest)
		}
	case <-time.After(time.Second):
		t.Fatal("selector did not receive the local message category")
	}
}

func TestServiceCategoryIgnoresExcludedCanaryContent(t *testing.T) {
	t.Parallel()

	categories := make(chan replies.Category, 1)
	selector := selectorFunc{selectBatch: func(_ string, category replies.Category) (replies.Batch, error) {
		categories <- category
		return replies.Batch{Selections: fixedSelections()}, nil
	}}
	service, err := NewService(42, 99, "Bosun", selector, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { closeService(t, service) })
	guildID := snowflake.ID(42)
	canary := "Bosun spokojna wiadomość <@123> <#456> <:idiota:789> https://idiota.example/?!!! DiScOrD.Gg/idiota!!! DiScOrD.CoM/InViTe/idiota!!! `idiota?` 123456"
	service.handleMessage(&guildID, 2, discord.Message{
		ID: 1, GuildID: &guildID, ChannelID: 2, Author: discord.User{ID: 7}, Content: canary,
	}, &recordingSender{})
	select {
	case category := <-categories:
		if category != replies.CategoryGeneric {
			t.Fatalf("selector category = %q, want %q", category, replies.CategoryGeneric)
		}
	case <-time.After(time.Second):
		t.Fatal("selector did not receive the local message category")
	}
}

func TestDynamicMessageUsesOneBatchAndOneModelDecision(t *testing.T) {
	t.Parallel()

	var batchCalls atomic.Int32
	committed := make(chan int, 1)
	selector := selectorFunc{selectBatch: func(_ string, _ replies.Category) (replies.Batch, error) {
		batchCalls.Add(1)
		return replies.Batch{Selections: fixedSelections()}, nil
	}, commitBatch: func(batch replies.Batch, choice int) (replies.Selection, error) {
		committed <- choice
		return batch.Selections[choice], nil
	}}
	var modelCalls atomic.Int32
	modelCalled := make(chan struct{}, 1)
	model := replyModelFunc{complete: func(context.Context, openrouter.Input) (int, error) {
		modelCalls.Add(1)
		modelCalled <- struct{}{}
		return 4, nil
	}}
	service, err := NewService(
		42, 99, "Bosun", selector, slog.New(slog.NewTextHandler(io.Discard, nil)), WithReplyModel(model),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { closeService(t, service) })
	guildID := snowflake.ID(42)
	service.handleMessage(&guildID, 2, discord.Message{
		ID: 1, GuildID: &guildID, ChannelID: 2, Author: discord.User{ID: 7}, Content: "Bosun, odpowiedz",
	}, &recordingSender{})
	select {
	case choice := <-committed:
		if choice != 4 {
			t.Fatalf("committed choice = %d, want 4", choice)
		}
	case <-time.After(time.Second):
		t.Fatal("the model choice was not committed")
	}
	if batchCalls.Load() != 1 || modelCalls.Load() != 1 {
		t.Fatalf("calls: batch = %d, model = %d", batchCalls.Load(), modelCalls.Load())
	}
}

func TestBlockedMessageSendDoesNotDelayApplicationCommand(t *testing.T) {
	t.Parallel()

	executor, err := newExecutor(1, 1)
	if err != nil {
		t.Fatalf("newExecutor() error = %v", err)
	}
	service, err := newService(42, 99, "Bosun", fixedSelector{}, slog.New(slog.NewTextHandler(io.Discard, nil)), executor, nil)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	sender := &blockingSender{started: make(chan struct{}), release: make(chan struct{})}
	t.Cleanup(func() {
		sender.unblock()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = service.Close(ctx)
	})

	guildID := snowflake.ID(42)
	service.handleMessage(&guildID, 2, discord.Message{
		ID:        1,
		GuildID:   &guildID,
		ChannelID: 2,
		Author:    discord.User{ID: 7},
		Content:   "Bosun",
	}, sender)
	select {
	case <-sender.started:
	case <-time.After(time.Second):
		t.Fatal("message send did not start")
	}

	acknowledged := make(chan discord.InteractionResponseType, 1)
	event := testAskEventWithResponder(t, func(responseType discord.InteractionResponseType, _ discord.InteractionResponseData, _ ...rest.RequestOpt) error {
		acknowledged <- responseType
		return nil
	})
	edits := make(chan discord.MessageUpdate, 1)
	service.handleApplicationCommand(event, interactionEditorFunc(func(_ context.Context, _ snowflake.ID, _ string, update discord.MessageUpdate) error {
		edits <- update
		return nil
	}))
	select {
	case responseType := <-acknowledged:
		if responseType != discord.InteractionResponseTypeDeferredCreateMessage {
			t.Fatalf("response type = %v, want deferred message", responseType)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("blocked message send delayed the interaction acknowledgement")
	}
	select {
	case update := <-edits:
		assertAllowedMentionsDisabled(t, update.AllowedMentions)
	case <-time.After(100 * time.Millisecond):
		t.Fatal("blocked message send delayed the interaction edit")
	}
}

func TestServiceCloseCancelsBothExecutors(t *testing.T) {
	t.Parallel()

	service, err := newService(
		42,
		99,
		"Bosun",
		fixedSelector{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		mustExecutor(t, 1, 1),
		mustExecutor(t, 1, 1),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	messageStarted, messageCanceled := submitCancellationProbe(t, service.messages)
	interactionStarted, interactionCanceled := submitCancellationProbe(t, service.interactions)
	<-messageStarted
	<-interactionStarted

	closeService(t, service)
	select {
	case <-messageCanceled:
	default:
		t.Fatal("message executor did not cancel its active job")
	}
	select {
	case <-interactionCanceled:
	default:
		t.Fatal("interaction executor did not cancel its active job")
	}
}

func TestServiceCloseCancelsExecutorsConcurrently(t *testing.T) {
	messages := mustExecutor(t, 1, 1)
	interactions := mustExecutor(t, 1, 1)
	messageStarted := make(chan struct{})
	releaseMessage := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseMessage) }) })
	if !messages.Submit(func(context.Context) {
		close(messageStarted)
		<-releaseMessage
	}) {
		t.Fatal("the message executor rejected the blocker")
	}
	interactionStarted, interactionCanceled := submitCancellationProbe(t, interactions)
	<-messageStarted
	<-interactionStarted

	service, err := newService(
		42,
		99,
		"Bosun",
		fixedSelector{},
		slog.New(slog.NewTextHandler(io.Discard, nil)),
		messages,
		interactions,
	)
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	closed := make(chan error, 1)
	go func() { closed <- service.Close(ctx) }()
	select {
	case <-interactionCanceled:
	case <-time.After(time.Second):
		t.Fatal("message shutdown delayed interaction cancellation")
	}
	releaseOnce.Do(func() { close(releaseMessage) })
	if err := <-closed; err != nil {
		t.Fatalf("Service.Close() error = %v", err)
	}
}

type fixedSelector struct{}

func (fixedSelector) SelectBatch(string, replies.Category) (replies.Batch, error) {
	return replies.Batch{Selections: fixedSelections()}, nil
}

func (fixedSelector) CommitBatch(batch replies.Batch, choice int) (replies.Selection, error) {
	return batch.Selections[choice], nil
}

func (fixedSelector) CancelBatch(replies.Batch) error {
	return nil
}

type selectorFunc struct {
	selectBatch func(string, replies.Category) (replies.Batch, error)
	commitBatch func(replies.Batch, int) (replies.Selection, error)
	cancelBatch func(replies.Batch) error
}

func (f selectorFunc) SelectBatch(userID string, category replies.Category) (replies.Batch, error) {
	return f.selectBatch(userID, category)
}

func (f selectorFunc) CommitBatch(batch replies.Batch, choice int) (replies.Selection, error) {
	if f.commitBatch != nil {
		return f.commitBatch(batch, choice)
	}
	return batch.Selections[choice], nil
}

func (f selectorFunc) CancelBatch(batch replies.Batch) error {
	if f.cancelBatch != nil {
		return f.cancelBatch(batch)
	}
	return nil
}

func fixedSelection() replies.Selection {
	return replies.Selection{
		Reply: replies.Reply{ID: "test-reply", Tier: replies.TierSharp, Text: "private reply text"},
		Tier:  replies.TierSharp,
	}
}

func fixedSelections() [replies.CandidateCount]replies.Selection {
	selections := [replies.CandidateCount]replies.Selection{}
	texts := [8]string{
		"private reply text",
		"pierwsza lokalna odpowiedź",
		"druga lokalna odpowiedź",
		"trzecia lokalna odpowiedź",
		"czwarta lokalna odpowiedź",
		"piąta lokalna odpowiedź",
		"szósta lokalna odpowiedź",
		"siódma lokalna odpowiedź",
	}
	for index := range selections {
		selections[index] = replies.Selection{
			Reply: replies.Reply{ID: "test-reply-" + string(rune('a'+index)), Tier: replies.TierSharp, Text: texts[index]},
			Tier:  replies.TierSharp,
		}
	}
	return selections
}

type blockingSender struct {
	started     chan struct{}
	release     chan struct{}
	startedOnce sync.Once
	releaseOnce sync.Once
}

func (s *blockingSender) CreateMessage(_ snowflake.ID, _ discord.MessageCreate, _ ...rest.RequestOpt) (*discord.Message, error) {
	s.startedOnce.Do(func() { close(s.started) })
	<-s.release
	return &discord.Message{}, nil
}

func (s *blockingSender) unblock() {
	s.releaseOnce.Do(func() { close(s.release) })
}

func testAskEventWithResponder(t *testing.T, respond events.InteractionResponderFunc) *events.ApplicationCommandInteractionCreate {
	t.Helper()

	interaction, err := discord.UnmarshalInteraction([]byte(`{
		"id":"111111111111111111",
		"application_id":"99",
		"type":2,
		"token":"interaction-token",
		"version":1,
		"guild_id":"42",
		"channel":{"id":"2","type":0},
		"member":{"user":{"id":"8","username":"friend","discriminator":"0"},"roles":[]},
		"data":{"id":"222222222222222222","name":"ask","type":1,"options":[{"name":"question","type":3,"value":"co teraz?"}]}
	}`))
	if err != nil {
		t.Fatalf("UnmarshalInteraction() error = %v", err)
	}
	command, ok := interaction.(discord.ApplicationCommandInteraction)
	if !ok {
		t.Fatalf("interaction type = %T", interaction)
	}
	return &events.ApplicationCommandInteractionCreate{
		ApplicationCommandInteraction: command,
		Respond:                       respond,
	}
}

func mustExecutor(t *testing.T, workers int, capacity int) *executor {
	t.Helper()
	executor, err := newExecutor(workers, capacity)
	if err != nil {
		t.Fatalf("newExecutor() error = %v", err)
	}
	return executor
}

func closeService(t *testing.T, service *Service) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := service.Close(ctx); err != nil {
		t.Fatalf("Service.Close() error = %v", err)
	}
}

func submitCancellationProbe(t *testing.T, executor *executor) (<-chan struct{}, <-chan struct{}) {
	t.Helper()
	started := make(chan struct{})
	canceled := make(chan struct{})
	if !executor.Submit(func(ctx context.Context) {
		close(started)
		<-ctx.Done()
		close(canceled)
	}) {
		t.Fatal("Submit() rejected a cancellation probe")
	}
	return started, canceled
}
