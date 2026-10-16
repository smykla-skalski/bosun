package bot

import (
	"testing"

	"github.com/disgoorg/disgo/discord"
)

func TestCommandsDefinesGuildAskCommand(t *testing.T) {
	t.Parallel()

	commands := Commands()
	if len(commands) != 1 {
		t.Fatalf("Commands() length = %d, want 1", len(commands))
	}
	command, ok := commands[0].(discord.SlashCommandCreate)
	if !ok {
		t.Fatalf("Commands()[0] type = %T", commands[0])
	}
	if command.Name != "ask" {
		t.Fatalf("command name = %q, want ask", command.Name)
	}
	if len(command.Options) != 1 {
		t.Fatalf("command options length = %d, want 1", len(command.Options))
	}
	option, ok := command.Options[0].(discord.ApplicationCommandOptionString)
	if !ok || option.Name != "question" || !option.Required {
		t.Fatalf("question option = %#v", command.Options[0])
	}
}
