package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/disgoorg/snowflake/v2"

	"github.com/smykla-skalski/bosun/internal/openrouter"
)

const (
	DefaultBotHandle                 = "Bosun"
	DefaultPort                      = 8080
	DefaultOpenRouterModel           = openrouter.ModelJev
	DefaultOpenRouterBalanceCooldown = 15 * time.Minute
	DefaultOpenRouterRequestTimeout  = 5 * time.Second
)

type Config struct {
	DiscordToken              string
	ApplicationID             snowflake.ID
	GuildID                   snowflake.ID
	BotHandle                 string
	Port                      int
	LogLevel                  slog.Level
	OpenRouterAPIKey          string
	OpenRouterModel           string
	OpenRouterBalanceCooldown time.Duration
	OpenRouterRequestTimeout  time.Duration
}

type LookupEnv func(string) (string, bool)

func Load() (Config, error) {
	return FromEnv(os.LookupEnv)
}

func FromEnv(lookup LookupEnv) (Config, error) {
	if lookup == nil {
		return Config{}, errors.New("environment lookup is nil")
	}

	token, err := required(lookup, "DISCORD_TOKEN")
	if err != nil {
		return Config{}, err
	}
	applicationID, err := requiredSnowflake(lookup, "DISCORD_APPLICATION_ID")
	if err != nil {
		return Config{}, err
	}
	guildID, err := requiredSnowflake(lookup, "DISCORD_GUILD_ID")
	if err != nil {
		return Config{}, err
	}
	port, err := readPort(lookup)
	if err != nil {
		return Config{}, err
	}
	level, err := readLogLevel(lookup)
	if err != nil {
		return Config{}, err
	}
	openRouterCooldown, err := readPositiveDuration(lookup, "OPENROUTER_BALANCE_COOLDOWN", DefaultOpenRouterBalanceCooldown)
	if err != nil {
		return Config{}, err
	}
	openRouterTimeout, err := readPositiveDuration(lookup, "OPENROUTER_REQUEST_TIMEOUT", DefaultOpenRouterRequestTimeout)
	if err != nil {
		return Config{}, err
	}

	handle := DefaultBotHandle
	if value, ok := lookup("BOT_HANDLE"); ok && strings.TrimSpace(value) != "" {
		handle = strings.TrimSpace(value)
	}
	openRouterModel := DefaultOpenRouterModel
	if value, ok := lookup("OPENROUTER_MODEL"); ok && strings.TrimSpace(value) != "" {
		openRouterModel = strings.TrimSpace(value)
	}
	if !openrouter.IsAllowedModel(openRouterModel) {
		return Config{}, errors.New("OPENROUTER_MODEL is not allowlisted")
	}
	openRouterAPIKey := ""
	if value, ok := lookup("OPENROUTER_API_KEY"); ok {
		openRouterAPIKey = strings.TrimSpace(value)
	}

	return Config{
		DiscordToken:              token,
		ApplicationID:             applicationID,
		GuildID:                   guildID,
		BotHandle:                 handle,
		Port:                      port,
		LogLevel:                  level,
		OpenRouterAPIKey:          openRouterAPIKey,
		OpenRouterModel:           openRouterModel,
		OpenRouterBalanceCooldown: openRouterCooldown,
		OpenRouterRequestTimeout:  openRouterTimeout,
	}, nil
}

func (c Config) OpenRouterEnabled() bool {
	return c.OpenRouterAPIKey != ""
}

func (c Config) Address() string {
	return fmt.Sprintf(":%d", c.Port)
}

func required(lookup LookupEnv, name string) (string, error) {
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func requiredSnowflake(lookup LookupEnv, name string) (snowflake.ID, error) {
	value, err := required(lookup, name)
	if err != nil {
		return 0, err
	}
	parsed, parseErr := strconv.ParseUint(value, 10, 64)
	if parseErr != nil || parsed == 0 {
		return 0, fmt.Errorf("%s must be a nonzero Discord ID", name)
	}
	return snowflake.ID(parsed), nil
}

func readPort(lookup LookupEnv) (int, error) {
	value, ok := lookup("PORT")
	if !ok || strings.TrimSpace(value) == "" {
		return DefaultPort, nil
	}
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return 0, errors.New("PORT must be a number from 1 through 65535")
	}
	return port, nil
}

func readLogLevel(lookup LookupEnv) (slog.Level, error) {
	value, ok := lookup("LOG_LEVEL")
	if !ok || strings.TrimSpace(value) == "" {
		return slog.LevelInfo, nil
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(strings.TrimSpace(value))); err != nil {
		return 0, errors.New("LOG_LEVEL must be debug, info, warn, or error")
	}
	return level, nil
}

func readPositiveDuration(lookup LookupEnv, name string, fallback time.Duration) (time.Duration, error) {
	value, ok := lookup(name)
	if !ok || strings.TrimSpace(value) == "" {
		return fallback, nil
	}
	duration, err := time.ParseDuration(strings.TrimSpace(value))
	if err != nil || duration <= 0 {
		return 0, fmt.Errorf("%s must be a positive duration", name)
	}
	return duration, nil
}
