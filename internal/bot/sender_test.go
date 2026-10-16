package bot

import (
	"context"
	"errors"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"
)

func TestSendReplyFallsBackOnlyForUnknownSourceMessage(t *testing.T) {
	t.Parallel()

	sender := &recordingSender{errors: []error{
		&rest.Error{Code: rest.JSONErrorCodeUnknownMessage},
		nil,
	}}
	original := discord.Message{ID: 1, ChannelID: 2}
	if err := sendReply(context.Background(), sender, 2, ReplyMessage(original, "reply")); err != nil {
		t.Fatalf("sendReply() error = %v", err)
	}
	if len(sender.messages) != 2 {
		t.Fatalf("send count = %d, want 2", len(sender.messages))
	}
	if sender.messages[0].MessageReference == nil {
		t.Fatal("first send does not reference the source message")
	}
	if sender.messages[1].MessageReference != nil {
		t.Fatal("fallback send still references the source message")
	}
	assertMentionsDisabled(t, sender.messages[1])
}

func TestSendReplyDoesNotRetryAmbiguousError(t *testing.T) {
	t.Parallel()

	transportError := errors.New("transport result is unknown")
	sender := &recordingSender{errors: []error{transportError}}
	err := sendReply(context.Background(), sender, 2, ReplyMessage(discord.Message{ID: 1, ChannelID: 2}, "reply"))
	if !errors.Is(err, transportError) {
		t.Fatalf("sendReply() error = %v, want transport error", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("send count = %d, want 1", len(sender.messages))
	}
}

func TestSendReplyReturnsFallbackError(t *testing.T) {
	t.Parallel()

	fallbackError := errors.New("fallback failed")
	sender := &recordingSender{errors: []error{
		&rest.Error{Code: rest.JSONErrorCodeUnknownMessage},
		fallbackError,
	}}
	err := sendReply(context.Background(), sender, 2, ReplyMessage(discord.Message{ID: 1, ChannelID: 2}, "reply"))
	if !errors.Is(err, fallbackError) {
		t.Fatalf("sendReply() error = %v, want fallback error", err)
	}
}

type recordingSender struct {
	messages []discord.MessageCreate
	errors   []error
}

func (s *recordingSender) CreateMessage(_ snowflake.ID, message discord.MessageCreate, _ ...rest.RequestOpt) (*discord.Message, error) {
	s.messages = append(s.messages, message)
	index := len(s.messages) - 1
	if index < len(s.errors) && s.errors[index] != nil {
		return nil, s.errors[index]
	}
	return &discord.Message{}, nil
}
