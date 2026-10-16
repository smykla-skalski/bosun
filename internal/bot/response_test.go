package bot

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestReplyMessageDisablesMentionsAndReferencesOriginal(t *testing.T) {
	t.Parallel()

	guildID := snowflake.ID(42)
	original := discord.Message{ID: 1, ChannelID: 2, GuildID: &guildID}
	got := ReplyMessage(original, "reply")
	assertMentionsDisabled(t, got)
	if got.MessageReference == nil || got.MessageReference.MessageID == nil || *got.MessageReference.MessageID != original.ID {
		t.Fatal("ReplyMessage() does not reference the original message")
	}
	if got.MessageReference.ChannelID == nil || *got.MessageReference.ChannelID != original.ChannelID {
		t.Fatal("ReplyMessage() has the wrong channel reference")
	}
	if got.MessageReference.GuildID == nil || *got.MessageReference.GuildID != guildID {
		t.Fatal("ReplyMessage() has the wrong guild reference")
	}
	if !got.MessageReference.FailIfNotExists {
		t.Fatal("ReplyMessage() permits an omitted source message")
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	if !strings.Contains(string(encoded), `"fail_if_not_exists":true`) {
		t.Fatalf("ReplyMessage() JSON omits fail_if_not_exists: %s", encoded)
	}
}

func TestInteractionUpdateDisablesMentions(t *testing.T) {
	t.Parallel()

	update := InteractionUpdate("reply")
	if update.Content == nil || *update.Content != "reply" {
		t.Fatal("InteractionUpdate() has the wrong content")
	}
	assertAllowedMentionsDisabled(t, update.AllowedMentions)
}

func assertMentionsDisabled(t *testing.T, message discord.MessageCreate) {
	t.Helper()
	assertAllowedMentionsDisabled(t, message.AllowedMentions)
}

func assertAllowedMentionsDisabled(t *testing.T, mentions *discord.AllowedMentions) {
	t.Helper()

	if mentions == nil {
		t.Fatal("AllowedMentions is nil")
	}
	if mentions.Parse == nil || mentions.Roles == nil || mentions.Users == nil {
		t.Fatal("AllowedMentions has a nil collection")
	}
	if len(mentions.Parse) != 0 || len(mentions.Roles) != 0 || len(mentions.Users) != 0 || mentions.RepliedUser {
		t.Fatalf("AllowedMentions permits a mention: %#v", *mentions)
	}
}
