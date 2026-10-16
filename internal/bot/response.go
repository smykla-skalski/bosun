package bot

import (
	"github.com/disgoorg/disgo/discord"
	"github.com/disgoorg/snowflake/v2"
)

func ReplyMessage(original discord.Message, content string) discord.MessageCreate {
	messageID := original.ID
	channelID := original.ChannelID
	return discord.MessageCreate{
		Content:         content,
		AllowedMentions: disabledMentions(),
		MessageReference: &discord.MessageReference{
			MessageID:       &messageID,
			ChannelID:       &channelID,
			GuildID:         copySnowflake(original.GuildID),
			FailIfNotExists: true,
		},
	}
}

func InteractionUpdate(content string) discord.MessageUpdate {
	return discord.MessageUpdate{
		Content:         &content,
		AllowedMentions: disabledMentions(),
	}
}

func disabledMentions() *discord.AllowedMentions {
	return &discord.AllowedMentions{
		Parse:       []discord.AllowedMentionType{},
		Roles:       []snowflake.ID{},
		Users:       []snowflake.ID{},
		RepliedUser: false,
	}
}

func copySnowflake(id *snowflake.ID) *snowflake.ID {
	if id == nil {
		return nil
	}
	copy := *id
	return &copy
}
