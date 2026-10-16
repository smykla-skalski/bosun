package main

import (
	"testing"
	"time"

	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/disgo/rest"
)

func TestEventManagerStartsSecondAskWhileFirstAcknowledgementBlocks(t *testing.T) {
	t.Parallel()

	firstStarted := make(chan struct{})
	secondStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	defer close(releaseFirst)
	listener := bot.NewListenerFunc(func(event *events.ApplicationCommandInteractionCreate) {
		_ = event.DeferCreateMessage(false)
	})
	manager := bot.NewEventManager(&bot.Client{}, eventManagerConfigOpts(listener)...)

	manager.DispatchEvent(askDispatchEvent(func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error {
		close(firstStarted)
		<-releaseFirst
		return nil
	}))
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("the first acknowledgement did not start")
	}

	manager.DispatchEvent(askDispatchEvent(func(discord.InteractionResponseType, discord.InteractionResponseData, ...rest.RequestOpt) error {
		close(secondStarted)
		return nil
	}))
	select {
	case <-secondStarted:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("the first acknowledgement blocked the second ask handler")
	}
}

func askDispatchEvent(respond events.InteractionResponderFunc) *events.ApplicationCommandInteractionCreate {
	return &events.ApplicationCommandInteractionCreate{Respond: respond}
}
