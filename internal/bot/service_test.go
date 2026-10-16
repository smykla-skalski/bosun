package bot

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

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

func TestServiceDoesNotLogErrorContent(t *testing.T) {
	t.Parallel()

	var output bytes.Buffer
	service := &Service{logger: slog.New(slog.NewJSONHandler(&output, nil))}
	service.logSelectionError(nil, 2, 7, TriggerHandle, errors.New("private message text"))
	if strings.Contains(output.String(), "private message text") {
		t.Fatal("selection error log contains private content")
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

type fixedSelector struct{}

func (fixedSelector) Select(string) (replies.Selection, error) {
	return replies.Selection{
		Reply: replies.Reply{ID: "test-reply", Tier: replies.TierSharp, Text: "private reply text"},
		Tier:  replies.TierSharp,
	}, nil
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
