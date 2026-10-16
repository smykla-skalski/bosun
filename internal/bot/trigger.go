package bot

import (
	"errors"
	"regexp"
	"strings"

	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

type TriggerType string

const (
	TriggerMention TriggerType = "mention"
	TriggerReply   TriggerType = "reply"
	TriggerHandle  TriggerType = "handle"
	TriggerAsk     TriggerType = "ask"
)

type Matcher struct {
	guildID snowflake.ID
	botID   snowflake.ID
	handle  *regexp.Regexp
}

func NewMatcher(guildID snowflake.ID, botID snowflake.ID, handle string) (*Matcher, error) {
	handle = strings.TrimSpace(handle)
	if guildID == 0 {
		return nil, errors.New("guild ID is zero")
	}
	if botID == 0 {
		return nil, errors.New("bot ID is zero")
	}
	if handle == "" {
		return nil, errors.New("bot handle is empty")
	}

	pattern := `(?i)(?:^|[^\pL\pN_])` + regexp.QuoteMeta(handle) + `(?:$|[^\pL\pN_])`
	compiled, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	return &Matcher{guildID: guildID, botID: botID, handle: compiled}, nil
}

func (m *Matcher) Match(guildID *snowflake.ID, message discord.Message) (TriggerType, bool) {
	if !m.accepts(guildID, message) {
		return "", false
	}
	for _, mention := range message.Mentions {
		if mention.ID == m.botID {
			return TriggerMention, true
		}
	}
	if message.ReferencedMessage != nil && message.ReferencedMessage.Author.ID == m.botID {
		return TriggerReply, true
	}
	if m.handle.MatchString(message.Content) {
		return TriggerHandle, true
	}
	return "", false
}

func (m *Matcher) accepts(guildID *snowflake.ID, message discord.Message) bool {
	if guildID == nil || *guildID != m.guildID {
		return false
	}
	if message.Author.Bot || message.Author.ID == m.botID || message.WebhookID != nil {
		return false
	}
	return true
}
