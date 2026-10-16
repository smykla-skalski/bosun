package bot

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	"github.com/smykla-skalski/bosun/internal/replies"
)

const (
	messageWorkerCount     = 4
	messageQueueCapacity   = 64
	interactionWorkerCount = 4
	interactionQueueSize   = 32
	interactionAckTimeout  = 2500 * time.Millisecond
	interactionBusyTimeout = 2 * time.Second
)

type ReplySelector interface {
	Select(userID string) (replies.Selection, error)
}

type Service struct {
	guildID      snowflake.ID
	botID        snowflake.ID
	matcher      *Matcher
	selector     ReplySelector
	messages     *executor
	interactions *executor
	logger       *slog.Logger
}

func NewService(guildID snowflake.ID, botID snowflake.ID, handle string, selector ReplySelector, logger *slog.Logger) (*Service, error) {
	return newService(guildID, botID, handle, selector, logger, nil, nil)
}

func newService(guildID snowflake.ID, botID snowflake.ID, handle string, selector ReplySelector, logger *slog.Logger, messages *executor, interactions *executor) (*Service, error) {
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
		messages:     messages,
		interactions: interactions,
		logger:       logger,
	}
	return service, nil
}

func (s *Service) OnMessageCreate(event *events.MessageCreate) {
	s.handleMessage(event.GuildID, event.ChannelID, event.Message, event.Client().Rest)
}

func (s *Service) Close(ctx context.Context) error {
	return errors.Join(s.messages.Close(ctx), s.interactions.Close(ctx))
}

func (s *Service) handleMessage(guildID *snowflake.ID, channelID snowflake.ID, message discord.Message, sender MessageSender) {
	trigger, ok := s.matcher.Match(guildID, message)
	if !ok {
		return
	}
	resolvedGuildID := *guildID
	accepted := s.messages.Submit(func(ctx context.Context) {
		selection, err := s.selector.Select(message.Author.ID.String())
		if err != nil {
			s.logSelectionError(&resolvedGuildID, channelID, message.Author.ID, trigger, err)
			return
		}
		response := ReplyMessage(message, selection.Reply.Text)
		err = sendReply(ctx, sender, channelID, response)
		if err != nil {
			s.logger.Error("discord message response failed",
				"guild_id", resolvedGuildID,
				"channel_id", channelID,
				"user_id", message.Author.ID,
				"trigger_type", trigger,
				"tier", selection.Tier,
				"reply_id", selection.Reply.ID,
				"error_type", fmt.Sprintf("%T", err),
			)
			return
		}
		s.logReply(resolvedGuildID, channelID, message.Author.ID, trigger, selection)
	})
	if !accepted {
		s.logger.Warn("discord message response dropped",
			"guild_id", resolvedGuildID,
			"channel_id", channelID,
			"user_id", message.Author.ID,
			"trigger_type", trigger,
			"reason", "message_queue_full",
		)
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

	selection, err := s.selector.Select(user.ID.String())
	channelID := interactionChannelID(event.Channel())
	if err != nil {
		s.logSelectionError(event.GuildID(), channelID, user.ID, TriggerAsk, err)
		return
	}

	guildID := *event.GuildID()
	ackCtx, cancel := context.WithTimeout(s.interactions.Context(), interactionAckTimeout)
	err = event.DeferCreateMessage(false, rest.WithCtx(ackCtx))
	cancel()
	if err != nil {
		s.logger.Error("discord interaction acknowledgement failed",
			"guild_id", guildID,
			"channel_id", channelID,
			"user_id", user.ID,
			"trigger_type", TriggerAsk,
			"tier", selection.Tier,
			"reply_id", selection.Reply.ID,
			"error_type", fmt.Sprintf("%T", err),
		)
		return
	}

	response := InteractionUpdate(selection.Reply.Text)
	applicationID := event.ApplicationID()
	token := event.Token()
	accepted := s.interactions.Submit(func(ctx context.Context) {
		if err := editor.Update(ctx, applicationID, token, response); err != nil {
			s.logger.Error("discord interaction response failed",
				"guild_id", guildID,
				"channel_id", channelID,
				"user_id", user.ID,
				"trigger_type", TriggerAsk,
				"tier", selection.Tier,
				"reply_id", selection.Reply.ID,
				"error_type", fmt.Sprintf("%T", err),
			)
			return
		}
		s.logReply(guildID, channelID, user.ID, TriggerAsk, selection)
	})
	if !accepted {
		s.logger.Warn("discord interaction queue overloaded",
			"guild_id", guildID,
			"channel_id", channelID,
			"user_id", user.ID,
			"trigger_type", TriggerAsk,
			"tier", selection.Tier,
			"reply_id", selection.Reply.ID,
			"reason", "interaction_queue_full",
		)
		busyCtx, busyCancel := context.WithTimeout(s.interactions.Context(), interactionBusyTimeout)
		err := editor.Update(busyCtx, applicationID, token, InteractionUpdate(interactionOverloadMessage))
		busyCancel()
		if err != nil {
			s.logger.Error("discord interaction overload response failed",
				"guild_id", guildID,
				"channel_id", channelID,
				"user_id", user.ID,
				"trigger_type", TriggerAsk,
				"tier", selection.Tier,
				"reply_id", selection.Reply.ID,
				"error_type", fmt.Sprintf("%T", err),
			)
		}
	}
}

func (s *Service) acceptsAsk(guildID *snowflake.ID, user discord.User, commandName string, question string, hasQuestion bool) bool {
	return guildID != nil && *guildID == s.guildID && commandName == "ask" && hasQuestion && strings.TrimSpace(question) != "" && !user.Bot && user.ID != s.botID
}

func (s *Service) logSelectionError(guildID *snowflake.ID, channelID snowflake.ID, userID snowflake.ID, trigger TriggerType, err error) {
	s.logger.Error("reply selection failed",
		"guild_id", guildID,
		"channel_id", channelID,
		"user_id", userID,
		"trigger_type", trigger,
		"error_type", fmt.Sprintf("%T", err),
	)
}

func (s *Service) logReply(guildID snowflake.ID, channelID snowflake.ID, userID snowflake.ID, trigger TriggerType, selection replies.Selection) {
	s.logger.Info("reply sent",
		"guild_id", guildID,
		"channel_id", channelID,
		"user_id", userID,
		"trigger_type", trigger,
		"tier", selection.Tier,
		"reply_id", selection.Reply.ID,
	)
}

func interactionChannelID(channel discord.InteractionChannel) snowflake.ID {
	if channel.MessageChannel == nil {
		return 0
	}
	return channel.ID()
}
