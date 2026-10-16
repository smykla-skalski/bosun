package bot

import "github.com/disgoorg/disgo/discord"

func Commands() []discord.ApplicationCommandCreate {
	return []discord.ApplicationCommandCreate{
		discord.SlashCommandCreate{
			Name:        "ask",
			Description: "Poproś Bosuna o odpowiedź",
			Options: []discord.ApplicationCommandOption{
				discord.ApplicationCommandOptionString{
					Name:        "question",
					Description: "Pytanie dla Bosuna",
					Required:    true,
				},
			},
		},
	}
}
