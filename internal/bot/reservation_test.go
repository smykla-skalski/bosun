package bot

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"slices"
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

type trackingSelector struct {
	mu          sync.Mutex
	selects     int
	commits     []int
	cancels     int
	outstanding int
}

func (s *trackingSelector) SelectBatch(string, replies.Category) (replies.Batch, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.selects++
	s.outstanding++
	return replies.Batch{Selections: fixedSelections()}, nil
}

func (s *trackingSelector) CommitBatch(batch replies.Batch, choice int) (replies.Selection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outstanding == 0 {
		return replies.Selection{}, errors.New("no reserved batch")
	}
	s.outstanding--
	s.commits = append(s.commits, choice)
	return batch.Selections[choice], nil
}

func (s *trackingSelector) CancelBatch(replies.Batch) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outstanding == 0 {
		return errors.New("no reserved batch")
	}
	s.outstanding--
	s.cancels++
	return nil
}

func (s *trackingSelector) state() (int, []int, int, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.selects, append([]int(nil), s.commits...), s.cancels, s.outstanding
}

type channelSender struct {
	messages chan discord.MessageCreate
	err      error
}

func (s *channelSender) CreateMessage(
	_ snowflake.ID,
	message discord.MessageCreate,
	_ ...rest.RequestOpt,
) (*discord.Message, error) {
	s.messages <- message
	return &discord.Message{}, s.err
}

func TestModelFaultCommitsOnlyCandidateZero(t *testing.T) {
	selector := &trackingSelector{}
	model := replyModelFunc{complete: func(context.Context, openrouter.Input) (int, error) {
		return 0, context.DeadlineExceeded
	}}
	sender := &channelSender{messages: make(chan discord.MessageCreate, 1)}
	service := newMessageReservationService(t, selector, model)
	sendTestMention(service, sender, 1)
	message := awaitSentMessage(t, sender.messages)
	if content := message.Content; content != fixedSelections()[0].Reply.Text {
		t.Fatalf("fallback content = %q", content)
	}
	assertReservationState(t, selector, 1, []int{0}, 0)
}

func TestSendFaultKeepsChosenCommitAndNoReservation(t *testing.T) {
	selector := &trackingSelector{}
	model := replyModelFunc{complete: func(context.Context, openrouter.Input) (int, error) { return 3, nil }}
	sender := &channelSender{messages: make(chan discord.MessageCreate, 1), err: errors.New("send fault")}
	service := newMessageReservationService(t, selector, model)
	sendTestMention(service, sender, 1)
	_ = awaitSentMessage(t, sender.messages)
	assertReservationState(t, selector, 1, []int{3}, 0)
}

func TestCommitFaultCancelsBatchBeforeReturn(t *testing.T) {
	canceled := make(chan struct{}, 1)
	selector := selectorFunc{
		selectBatch: func(string, replies.Category) (replies.Batch, error) {
			return replies.Batch{Selections: fixedSelections()}, nil
		},
		commitBatch: func(replies.Batch, int) (replies.Selection, error) {
			return replies.Selection{}, errors.New("commit fault")
		},
		cancelBatch: func(replies.Batch) error {
			canceled <- struct{}{}
			return nil
		},
	}
	model := replyModelFunc{complete: func(context.Context, openrouter.Input) (int, error) { return 5, nil }}
	sender := &channelSender{messages: make(chan discord.MessageCreate, 1)}
	service := newMessageReservationService(t, selector, model)
	sendTestMention(service, sender, 1)
	awaitTestSignal(t, canceled, "batch cancel")
	select {
	case <-sender.messages:
		t.Fatal("a reply was sent after the commit fault")
	default:
	}
}

func TestAckFaultCreatesNoReservation(t *testing.T) {
	selector := &trackingSelector{}
	service := newInteractionReservationService(t, selector, nil, nil)
	event := testAskEventWithResponder(t, func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error {
		return errors.New("ack fault")
	})
	service.handleApplicationCommand(event, interactionEditorFunc(func(context.Context, snowflake.ID, string, discord.MessageUpdate) error {
		t.Fatal("acknowledgement fault reached the editor")
		return nil
	}))
	assertReservationState(t, selector, 0, nil, 0)
}

func TestQueueRejectionCommitsCandidateZero(t *testing.T) {
	selector := &trackingSelector{}
	executor := mustExecutor(t, 1, 1)
	started := make(chan struct{})
	release := make(chan struct{})
	if !executor.Submit(func(context.Context) { close(started); <-release }) {
		t.Fatal("blocking job was rejected")
	}
	<-started
	if !executor.Submit(func(context.Context) {}) {
		t.Fatal("queued job was rejected")
	}
	service := newInteractionReservationService(t, selector, executor, nil)
	edited := make(chan struct{}, 1)
	service.handleApplicationCommand(successfulAskEvent(t), interactionEditorFunc(func(context.Context, snowflake.ID, string, discord.MessageUpdate) error {
		edited <- struct{}{}
		return nil
	}))
	close(release)
	awaitTestSignal(t, edited, "overload reply edit")
	assertReservationState(t, selector, 1, []int{0}, 0)
}

func TestShutdownCancelsActiveModelAndCommitsCandidateZero(t *testing.T) {
	selector := &trackingSelector{}
	started := make(chan struct{})
	type finalEdit struct {
		update      discord.MessageUpdate
		initialErr  error
		hasDeadline bool
	}
	edits := make(chan finalEdit, 2)
	model := replyModelFunc{complete: func(ctx context.Context, _ openrouter.Input) (int, error) {
		close(started)
		<-ctx.Done()
		return 0, ctx.Err()
	}}
	service := newInteractionReservationService(t, selector, nil, model)
	service.finalTimeout = 40 * time.Millisecond
	service.handleApplicationCommand(successfulAskEvent(t), interactionEditorFunc(func(ctx context.Context, _ snowflake.ID, _ string, update discord.MessageUpdate) error {
		_, hasDeadline := ctx.Deadline()
		edits <- finalEdit{update: update, initialErr: ctx.Err(), hasDeadline: hasDeadline}
		<-ctx.Done()
		return ctx.Err()
	}))
	awaitTestSignal(t, started, "model call")
	closeService(t, service)
	select {
	case edit := <-edits:
		if edit.initialErr != nil || !edit.hasDeadline {
			t.Fatalf("final edit context = error %v, deadline %v", edit.initialErr, edit.hasDeadline)
		}
		assertInteractionUpdate(t, edit.update, fixedSelections()[0].Reply.Text)
	default:
		t.Fatal("model cancellation did not produce a final update")
	}
	select {
	case <-edits:
		t.Fatal("model cancellation produced multiple final updates")
	default:
	}
	assertReservationState(t, selector, 1, []int{0}, 0)
}

func TestShutdownFinalizesQueuedInteractionWithoutReservation(t *testing.T) {
	selector := &trackingSelector{}
	executor := mustExecutor(t, 1, 1)
	started := make(chan struct{})
	if !executor.Submit(func(ctx context.Context) { close(started); <-ctx.Done() }) {
		t.Fatal("blocking job was rejected")
	}
	<-started
	service := newInteractionReservationService(t, selector, executor, nil)
	edits := make(chan discord.MessageUpdate, 2)
	service.handleApplicationCommand(successfulAskEvent(t), interactionEditorFunc(func(_ context.Context, _ snowflake.ID, _ string, update discord.MessageUpdate) error {
		edits <- update
		return nil
	}))
	closeService(t, service)
	assertOneFailureUpdate(t, edits)
	assertReservationState(t, selector, 0, nil, 0)
}

func TestShutdownFinalizesQueuedInteractionsConcurrently(t *testing.T) {
	const queuedInteractions = 8
	selector := &trackingSelector{}
	executor := mustExecutor(t, 1, queuedInteractions)
	workerStarted := make(chan struct{})
	if !executor.Submit(func(ctx context.Context) {
		close(workerStarted)
		<-ctx.Done()
	}) {
		t.Fatal("blocking job was rejected")
	}
	<-workerStarted

	service := newInteractionReservationService(t, selector, executor, nil)
	service.finalTimeout = 2 * time.Second
	editsStarted := make(chan struct{}, queuedInteractions)
	releaseEdits := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(releaseEdits) }) })
	var editCount atomic.Int32
	editor := interactionEditorFunc(func(ctx context.Context, _ snowflake.ID, _ string, _ discord.MessageUpdate) error {
		editCount.Add(1)
		editsStarted <- struct{}{}
		select {
		case <-releaseEdits:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	for range queuedInteractions {
		service.handleApplicationCommand(successfulAskEvent(t), editor)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	closed := make(chan error, 1)
	go func() { closed <- service.Close(ctx) }()
	for range queuedInteractions {
		select {
		case <-editsStarted:
		case <-time.After(time.Second):
			t.Fatal("queued final updates ran in series")
		}
	}
	releaseOnce.Do(func() { close(releaseEdits) })
	if err := <-closed; err != nil {
		t.Fatalf("Service.Close() error = %v", err)
	}
	if got := editCount.Load(); got != queuedInteractions {
		t.Fatalf("final update count = %d, want %d", got, queuedInteractions)
	}
	assertReservationState(t, selector, 0, nil, 0)
}

func TestServiceSendsTwentyOneActualRepliesWithoutRecentRepeat(t *testing.T) {
	catalog, err := replies.LoadEmbedded()
	if err != nil {
		t.Fatalf("LoadEmbedded() error = %v", err)
	}
	selector := replies.NewSelector(catalog, replies.WithIndex(func(int) int { return 0 }))
	model := replyModelFunc{complete: func(context.Context, openrouter.Input) (int, error) { return 0, nil }}
	sender := &channelSender{messages: make(chan discord.MessageCreate, 1)}
	service := newMessageReservationService(t, selector, model)
	recent := make([]string, 0, 21)
	for index := 1; index <= 21; index++ {
		sendTestMention(service, sender, snowflake.ID(index))
		content := awaitSentMessage(t, sender.messages).Content
		for _, prior := range recent {
			if content == prior {
				t.Fatalf("reply %d repeated recent text %q", index, content)
			}
		}
		recent = append(recent, content)
	}
}

func newMessageReservationService(t *testing.T, selector ReplySelector, model ReplyModel) *Service {
	t.Helper()
	service, err := NewService(
		42, 99, "Bosun", selector, slog.New(slog.NewTextHandler(io.Discard, nil)), WithReplyModel(model),
	)
	if err != nil {
		t.Fatalf("NewService() error = %v", err)
	}
	t.Cleanup(func() { closeService(t, service) })
	return service
}

func newInteractionReservationService(
	t *testing.T,
	selector ReplySelector,
	interactions *executor,
	model ReplyModel,
) *Service {
	t.Helper()
	service, err := newService(
		42, 99, "Bosun", selector, slog.New(slog.NewTextHandler(io.Discard, nil)),
		nil, interactions, WithReplyModel(model),
	)
	if err != nil {
		t.Fatalf("newService() error = %v", err)
	}
	t.Cleanup(func() { closeService(t, service) })
	return service
}

func sendTestMention(service *Service, sender MessageSender, messageID snowflake.ID) {
	guildID := snowflake.ID(42)
	service.handleMessage(&guildID, 2, discord.Message{
		ID: messageID, GuildID: &guildID, ChannelID: 2,
		Author: discord.User{ID: 7}, Content: "Bosun, odpowiedz.",
	}, sender)
}

func awaitSentMessage(t *testing.T, messages <-chan discord.MessageCreate) discord.MessageCreate {
	t.Helper()
	select {
	case message := <-messages:
		return message
	case <-time.After(5 * time.Second):
		t.Fatal("reply was not sent")
		return discord.MessageCreate{}
	}
}

func awaitTestSignal(t *testing.T, signal <-chan struct{}, name string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatalf("%s did not occur", name)
	}
}

func successfulAskEvent(t *testing.T) *events.ApplicationCommandInteractionCreate {
	t.Helper()
	return testAskEventWithResponder(t, func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error {
		return nil
	})
}

func assertReservationState(t *testing.T, selector *trackingSelector, selects int, commits []int, cancels int) {
	t.Helper()
	gotSelects, gotCommits, gotCancels, outstanding := selector.state()
	if gotSelects != selects || !slices.Equal(gotCommits, commits) || gotCancels != cancels || outstanding != 0 {
		t.Fatalf("reservation state = selects %d, commits %v, cancels %d, outstanding %d", gotSelects, gotCommits, gotCancels, outstanding)
	}
}
