package bot

import (
	"context"
	"errors"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

type MessageSender interface {
	CreateMessage(channelID snowflake.ID, message discord.MessageCreate, opts ...rest.RequestOpt) (*discord.Message, error)
}

func sendReply(ctx context.Context, sender MessageSender, channelID snowflake.ID, message discord.MessageCreate) error {
	_, err := sender.CreateMessage(channelID, message, rest.WithCtx(ctx))
	if err == nil || !isUnknownMessage(err) {
		return err
	}

	message.MessageReference = nil
	_, err = sender.CreateMessage(channelID, message, rest.WithCtx(ctx))
	return err
}

func isUnknownMessage(err error) bool {
	var discordError *rest.Error
	return errors.As(err, &discordError) && discordError.Code == rest.JSONErrorCodeUnknownMessage
}
