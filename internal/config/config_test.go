package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"
)

func TestFromEnvUsesDefaults(t *testing.T) {
	t.Parallel()

	got, err := FromEnv(testEnv(map[string]string{
		"DISCORD_TOKEN":          "secret-token",
		"DISCORD_APPLICATION_ID": "123",
		"DISCORD_GUILD_ID":       "456",
	}))
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if got.DiscordToken != "secret-token" {
		t.Fatal("FromEnv() changed the Discord token")
	}
	if got.ApplicationID != 123 || got.GuildID != 456 {
		t.Fatalf("FromEnv() IDs = %d, %d; want 123, 456", got.ApplicationID, got.GuildID)
	}
	if got.BotHandle != DefaultBotHandle || got.Port != DefaultPort || got.LogLevel != slog.LevelInfo {
		t.Fatalf("FromEnv() defaults = %q, %d, %s", got.BotHandle, got.Port, got.LogLevel)
	}
	if got.OpenRouterEnabled() {
		t.Fatal("FromEnv() enabled OpenRouter without an API key")
	}
	if got.OpenRouterModel != DefaultOpenRouterModel || got.OpenRouterBalanceCooldown != DefaultOpenRouterBalanceCooldown || got.OpenRouterRequestTimeout != DefaultOpenRouterRequestTimeout {
		t.Fatalf("FromEnv() OpenRouter defaults = %q, %v, %v", got.OpenRouterModel, got.OpenRouterBalanceCooldown, got.OpenRouterRequestTimeout)
	}
	if got.Address() != ":8080" {
		t.Fatalf("Address() = %q, want :8080", got.Address())
	}
}

func TestFromEnvReadsOptions(t *testing.T) {
	t.Parallel()

	got, err := FromEnv(testEnv(map[string]string{
		"DISCORD_TOKEN":               "secret-token",
		"DISCORD_APPLICATION_ID":      "123",
		"DISCORD_GUILD_ID":            "456",
		"BOT_HANDLE":                  "  Kapitan  ",
		"PORT":                        "9090",
		"LOG_LEVEL":                   "debug",
		"OPENROUTER_API_KEY":          "  openrouter-secret  ",
		"OPENROUTER_MODEL":            "  " + DefaultOpenRouterModel + "  ",
		"OPENROUTER_BALANCE_COOLDOWN": "30m",
		"OPENROUTER_REQUEST_TIMEOUT":  "3s",
	}))
	if err != nil {
		t.Fatalf("FromEnv() error = %v", err)
	}
	if got.BotHandle != "Kapitan" || got.Port != 9090 || got.LogLevel != slog.LevelDebug {
		t.Fatalf("FromEnv() options = %q, %d, %s", got.BotHandle, got.Port, got.LogLevel)
	}
	if !got.OpenRouterEnabled() || got.OpenRouterAPIKey != "openrouter-secret" || got.OpenRouterModel != DefaultOpenRouterModel {
		t.Fatalf("FromEnv() OpenRouter identity = enabled %v, key %q, model %q", got.OpenRouterEnabled(), got.OpenRouterAPIKey, got.OpenRouterModel)
	}
	if got.OpenRouterBalanceCooldown != 30*time.Minute || got.OpenRouterRequestTimeout != 3*time.Second {
		t.Fatalf("FromEnv() OpenRouter times = %v, %v", got.OpenRouterBalanceCooldown, got.OpenRouterRequestTimeout)
	}
}

func TestFromEnvRejectsMissingRequiredValues(t *testing.T) {
	t.Parallel()

	base := map[string]string{
		"DISCORD_TOKEN":          "secret-token",
		"DISCORD_APPLICATION_ID": "123",
		"DISCORD_GUILD_ID":       "456",
	}
	for _, name := range []string{"DISCORD_TOKEN", "DISCORD_APPLICATION_ID", "DISCORD_GUILD_ID"} {
		name := name
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			values := copyMap(base)
			delete(values, name)
			_, err := FromEnv(testEnv(values))
			if err == nil || !strings.Contains(err.Error(), name+" is required") {
				t.Fatalf("FromEnv() error = %v, want missing %s error", err, name)
			}
		})
	}
}

func TestFromEnvRejectsInvalidValuesWithoutSecretLeak(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		change map[string]string
		want   string
	}{
		{name: "application ID", change: map[string]string{"DISCORD_APPLICATION_ID": "not-an-id"}, want: "DISCORD_APPLICATION_ID"},
		{name: "zero guild ID", change: map[string]string{"DISCORD_GUILD_ID": "0"}, want: "DISCORD_GUILD_ID"},
		{name: "port", change: map[string]string{"PORT": "70000"}, want: "PORT"},
		{name: "log level", change: map[string]string{"LOG_LEVEL": "verbose"}, want: "LOG_LEVEL"},
		{name: "model allowlist", change: map[string]string{"OPENROUTER_MODEL": "private-person/private-model"}, want: "OPENROUTER_MODEL"},
		{name: "balance cooldown", change: map[string]string{"OPENROUTER_BALANCE_COOLDOWN": "never"}, want: "OPENROUTER_BALANCE_COOLDOWN"},
		{name: "zero request timeout", change: map[string]string{"OPENROUTER_REQUEST_TIMEOUT": "0s"}, want: "OPENROUTER_REQUEST_TIMEOUT"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			values := map[string]string{
				"DISCORD_TOKEN":          "do-not-log-this-secret",
				"DISCORD_APPLICATION_ID": "123",
				"DISCORD_GUILD_ID":       "456",
			}
			for key, value := range test.change {
				values[key] = value
			}
			_, err := FromEnv(testEnv(values))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("FromEnv() error = %v, want %s error", err, test.want)
			}
			if strings.Contains(err.Error(), values["DISCORD_TOKEN"]) {
				t.Fatal("FromEnv() error contains the Discord token")
			}
			if model := values["OPENROUTER_MODEL"]; model != "" && strings.Contains(err.Error(), model) {
				t.Fatal("FromEnv() error contains the rejected model")
			}
		})
	}
}

func testEnv(values map[string]string) LookupEnv {
	return func(name string) (string, bool) {
		value, ok := values[name]
		return value, ok
	}
}

func copyMap(source map[string]string) map[string]string {
	copy := make(map[string]string, len(source))
	for key, value := range source {
		copy[key] = value
	}
	return copy
}
