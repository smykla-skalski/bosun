package bot

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func TestMatcherRecognizesTriggers(t *testing.T) {
	t.Parallel()

	matcher := testMatcher(t)
	guildID := snowflake.ID(42)
	tests := []struct {
		name    string
		message discord.Message
		want    TriggerType
	}{
		{
			name: "actual mention",
			message: discord.Message{
				Author:   discord.User{ID: 7},
				Mentions: []discord.User{{ID: 99}},
				Content:  "hej",
			},
			want: TriggerMention,
		},
		{
			name: "reply to bot",
			message: discord.Message{
				Author:            discord.User{ID: 7},
				ReferencedMessage: &discord.Message{Author: discord.User{ID: 99}},
			},
			want: TriggerReply,
		},
		{
			name:    "plain lowercase handle",
			message: discord.Message{Author: discord.User{ID: 7}, Content: "bosun, masz chwilę?"},
			want:    TriggerHandle,
		},
		{
			name:    "plain uppercase handle",
			message: discord.Message{Author: discord.User{ID: 7}, Content: "Hej BOSUN!"},
			want:    TriggerHandle,
		},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			got, ok := matcher.Match(&guildID, test.message)
			if !ok || got != test.want {
				t.Fatalf("Match() = %q, %v; want %q, true", got, ok, test.want)
			}
		})
	}
}

func TestMatcherRejectsHandleSubstrings(t *testing.T) {
	t.Parallel()

	matcher := testMatcher(t)
	guildID := snowflake.ID(42)
	for _, content := range []string{"bosunship", "prebosun", "bosun_foo", "bosun2", "łbosun"} {
		content := content
		t.Run(content, func(t *testing.T) {
			t.Parallel()
			_, ok := matcher.Match(&guildID, discord.Message{Author: discord.User{ID: 7}, Content: content})
			if ok {
				t.Fatalf("Match() accepted substring %q", content)
			}
		})
	}
}

func TestMatcherFiltersUnsupportedMessages(t *testing.T) {
	t.Parallel()

	matcher := testMatcher(t)
	guildID := snowflake.ID(42)
	otherGuildID := snowflake.ID(43)
	webhookID := snowflake.ID(100)
	tests := []struct {
		name    string
		guildID *snowflake.ID
		message discord.Message
	}{
		{name: "direct message", message: discord.Message{Author: discord.User{ID: 7}, Content: "Bosun"}},
		{name: "other guild", guildID: &otherGuildID, message: discord.Message{Author: discord.User{ID: 7}, Content: "Bosun"}},
		{name: "bot author", guildID: &guildID, message: discord.Message{Author: discord.User{ID: 7, Bot: true}, Content: "Bosun"}},
		{name: "self author", guildID: &guildID, message: discord.Message{Author: discord.User{ID: 99}, Content: "Bosun"}},
		{name: "webhook", guildID: &guildID, message: discord.Message{Author: discord.User{ID: 7}, WebhookID: &webhookID, Content: "Bosun"}},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if _, ok := matcher.Match(test.guildID, test.message); ok {
				t.Fatal("Match() accepted a filtered message")
			}
		})
	}
}

func TestMatcherDoesNotTreatOtherMentionAsBotMention(t *testing.T) {
	t.Parallel()

	matcher := testMatcher(t)
	guildID := snowflake.ID(42)
	_, ok := matcher.Match(&guildID, discord.Message{
		Author:   discord.User{ID: 7},
		Mentions: []discord.User{{ID: 98}},
		Content:  "hej",
	})
	if ok {
		t.Fatal("Match() accepted a mention of another user")
	}
}

func testMatcher(t *testing.T) *Matcher {
	t.Helper()

	matcher, err := NewMatcher(42, 99, "Bosun")
	if err != nil {
		t.Fatalf("NewMatcher() error = %v", err)
	}
	return matcher
}
