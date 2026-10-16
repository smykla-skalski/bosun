package bot

import (
	"context"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

const interactionOverloadMessage = "Bosun ma teraz pełne ręce roboty. Spróbuj ponownie za chwilę."

type interactionEditor interface {
	Update(ctx context.Context, applicationID snowflake.ID, token string, update discord.MessageUpdate) error
}

type discordInteractionEditor struct {
	interactions rest.Interactions
}

func (e discordInteractionEditor) Update(ctx context.Context, applicationID snowflake.ID, token string, update discord.MessageUpdate) error {
	_, err := e.interactions.UpdateInteractionResponse(applicationID, token, update, rest.WithCtx(ctx))
	return err
}
