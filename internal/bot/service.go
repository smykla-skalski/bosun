package bot

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/smykla-skalski/bosun/internal/messageprofile"
	"github.com/smykla-skalski/bosun/internal/openrouter"
	"github.com/smykla-skalski/bosun/internal/replies"
)

const (
	messageWorkerCount      = 4
	messageQueueCapacity    = 64
	interactionWorkerCount  = 4
	interactionQueueSize    = 32
	interactionAckTimeout   = 2500 * time.Millisecond
	interactionFinalTimeout = 5 * time.Second
)

type ReplySelector interface {
	SelectBatch(userID string, category replies.Category) (replies.Batch, error)
	CommitBatch(batch replies.Batch, choice int) (replies.Selection, error)
	CancelBatch(batch replies.Batch) error
}

type ReplyModel interface {
	Complete(context.Context, openrouter.Input) (int, error)
}

type ServiceOption func(*Service)

func WithReplyModel(model ReplyModel) ServiceOption {
	return func(service *Service) {
		service.model = model
	}
}

type Service struct {
	guildID      snowflake.ID
	botID        snowflake.ID
	matcher      *Matcher
	selector     ReplySelector
	model        ReplyModel
	botHandle    string
	messages     *executor
	interactions *executor
	logger       *slog.Logger
	finalTimeout time.Duration
	finalUpdates finalUpdateLifecycle
}

type finalUpdateLifecycle struct {
	mu     sync.Mutex
	active int
	closed bool
	done   chan struct{}
}

func newFinalUpdateLifecycle() finalUpdateLifecycle {
	return finalUpdateLifecycle{done: make(chan struct{})}
}

func (l *finalUpdateLifecycle) acquire() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return false
	}
	l.active++
	return true
}

func (l *finalUpdateLifecycle) release() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.active--
	if l.closed && l.active == 0 {
		close(l.done)
	}
}

func (l *finalUpdateLifecycle) close(ctx context.Context) error {
	l.mu.Lock()
	if !l.closed {
		l.closed = true
		if l.active == 0 {
			close(l.done)
		}
	}
	done := l.done
	l.mu.Unlock()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return errExecutorShutdown
	}
}

func NewService(guildID snowflake.ID, botID snowflake.ID, handle string, selector ReplySelector, logger *slog.Logger, options ...ServiceOption) (*Service, error) {
	return newService(guildID, botID, handle, selector, logger, nil, nil, options...)
}

func newService(guildID snowflake.ID, botID snowflake.ID, handle string, selector ReplySelector, logger *slog.Logger, messages *executor, interactions *executor, options ...ServiceOption) (*Service, error) {
	if selector == nil {
		return nil, errors.New("reply selector is nil")
	}
	matcher, err := NewMatcher(guildID, botID, handle)
	if err != nil {
		return nil, err
	}
	if logger == nil {
		logger = slog.Default()
	}
	if messages == nil {
		messages, err = newExecutor(messageWorkerCount, messageQueueCapacity)
		if err != nil {
			return nil, err
		}
	}
	if interactions == nil {
		interactions, err = newExecutor(interactionWorkerCount, interactionQueueSize)
		if err != nil {
			_ = messages.Close(context.Background())
			return nil, err
		}
	}
	service := &Service{
		guildID:      guildID,
		botID:        botID,
		matcher:      matcher,
		selector:     selector,
		botHandle:    strings.TrimSpace(handle),
		messages:     messages,
		interactions: interactions,
		logger:       logger,
		finalTimeout: interactionFinalTimeout,
		finalUpdates: newFinalUpdateLifecycle(),
	}
	for _, option := range options {
		option(service)
	}
	return service, nil
}

func (s *Service) OnMessageCreate(event *events.MessageCreate) {
	s.handleMessage(event.GuildID, event.ChannelID, event.Message, event.Client().Rest)
}

func (s *Service) Close(ctx context.Context) error {
	var shutdown sync.WaitGroup
	var messageErr error
	var interactionErr error
	var finalUpdateErr error
	shutdown.Add(3)
	go func() {
		defer shutdown.Done()
		messageErr = s.messages.Close(ctx)
	}()
	go func() {
		defer shutdown.Done()
		interactionErr = s.interactions.Close(ctx)
	}()
	go func() {
		defer shutdown.Done()
		finalUpdateErr = s.finalUpdates.close(ctx)
	}()
	shutdown.Wait()
	return errors.Join(messageErr, interactionErr, finalUpdateErr)
}

func (s *Service) handleMessage(guildID *snowflake.ID, channelID snowflake.ID, message discord.Message, sender MessageSender) {
	trigger, ok := s.matcher.Match(guildID, message)
	if !ok {
		return
	}
	accepted := s.messages.Submit(func(ctx context.Context) {
		category := replies.Classify(messageprofile.Clean(message.Content, s.botHandle))
		batch, err := s.selector.SelectBatch(message.Author.ID.String(), category)
		if err != nil {
			s.logFault(replies.Selection{}, staticResult("", "selection"))
			return
		}
		result := s.resolveReply(ctx, message.Content, trigger, category, batch.Selections)
		selection, err := s.commitSelection(batch, result.selectionIndex)
		if err != nil {
			s.logFault(replies.Selection{}, result.withFault("selection_commit"))
			return
		}
		result.text = selection.Reply.Text
		response := ReplyMessage(message, result.text)
		err = sendReply(ctx, sender, channelID, response)
		if err != nil {
			s.logFault(selection, result.withFault("discord_send"))
			return
		}
		s.logResult(selection, result)
	})
	if !accepted {
		s.logFault(replies.Selection{}, staticResult("", "local_overload"))
	}
}

func (s *Service) OnApplicationCommand(event *events.ApplicationCommandInteractionCreate) {
	s.handleApplicationCommand(event, discordInteractionEditor{interactions: event.Client().Rest})
}

func (s *Service) handleApplicationCommand(event *events.ApplicationCommandInteractionCreate, editor interactionEditor) {
	data, ok := event.Data.(discord.SlashCommandInteractionData)
	if !ok {
		return
	}
	question, hasQuestion := data.OptString("question")
	user := event.User()
	if !s.acceptsAsk(event.GuildID(), user, data.CommandName(), question, hasQuestion) {
		return
	}
	if !s.finalUpdates.acquire() {
		return
	}

	category := replies.Classify(messageprofile.Clean(question, s.botHandle))
	ackCtx, cancel := context.WithTimeout(s.interactions.Context(), interactionAckTimeout)
	err := event.DeferCreateMessage(false, rest.WithCtx(ackCtx))
	cancel()
	if err != nil {
		s.finalUpdates.release()
		s.logFault(replies.Selection{}, staticResult("", "discord_ack"))
		return
	}

	applicationID := event.ApplicationID()
	token := event.Token()
	accepted := s.interactions.SubmitWithDrop(func(ctx context.Context) {
		defer s.finalUpdates.release()
		batch, err := s.selector.SelectBatch(user.ID.String(), category)
		if err != nil {
			s.finishInteractionFault(ctx, editor, applicationID, token, "selection")
			return
		}
		result := s.resolveReply(ctx, question, TriggerAsk, category, batch.Selections)
		selection, err := s.commitSelection(batch, result.selectionIndex)
		if err != nil {
			s.finishInteractionFault(ctx, editor, applicationID, token, "selection_commit")
			return
		}
		result.text = selection.Reply.Text
		response := InteractionUpdate(result.text)
		if err := s.editFinalInteraction(ctx, editor, applicationID, token, response); err != nil {
			s.logFault(selection, result.withFault("discord_edit"))
			return
		}
		s.logResult(selection, result)
	}, func() {
		defer s.finalUpdates.release()
		s.finishInteractionFault(context.Background(), editor, applicationID, token, "service_shutdown")
	})
	if !accepted {
		defer s.finalUpdates.release()
		batch, selectErr := s.selector.SelectBatch(user.ID.String(), category)
		if selectErr != nil {
			s.finishInteractionFault(context.Background(), editor, applicationID, token, "selection")
			return
		}
		fallback := batch.Selections[0]
		result := staticResult(fallback.Reply.Text, "local_overload")
		selection, commitErr := s.commitSelection(batch, 0)
		if commitErr != nil {
			s.finishInteractionFault(context.Background(), editor, applicationID, token, "selection_commit")
			return
		}
		result.text = selection.Reply.Text
		s.logFault(selection, result)
		err := s.editFinalInteraction(
			s.interactions.Context(), editor, applicationID, token, InteractionUpdate(result.text),
		)
		if err != nil {
			s.logFault(selection, result.withFault("discord_edit"))
		}
	}
}

func (s *Service) finishInteractionFault(
	ctx context.Context,
	editor interactionEditor,
	applicationID snowflake.ID,
	token string,
	faultClass string,
) {
	result := staticResult(interactionFailureMessage, faultClass)
	if err := s.editFinalInteraction(ctx, editor, applicationID, token, InteractionUpdate(result.text)); err != nil {
		s.logFault(replies.Selection{}, result.withFault("discord_edit"))
		return
	}
	s.logFault(replies.Selection{}, result)
}

func (s *Service) editFinalInteraction(
	parent context.Context,
	editor interactionEditor,
	applicationID snowflake.ID,
	token string,
	update discord.MessageUpdate,
) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), s.finalTimeout)
	defer cancel()
	return editor.Update(ctx, applicationID, token, update)
}

func (s *Service) commitSelection(batch replies.Batch, choice int) (replies.Selection, error) {
	selection, err := s.selector.CommitBatch(batch, choice)
	if err != nil {
		_ = s.selector.CancelBatch(batch)
		return replies.Selection{}, err
	}
	return selection, nil
}

func (s *Service) acceptsAsk(guildID *snowflake.ID, user discord.User, commandName string, question string, hasQuestion bool) bool {
	return guildID != nil && *guildID == s.guildID && commandName == "ask" && hasQuestion && strings.TrimSpace(question) != "" && !user.Bot && user.ID != s.botID
}
