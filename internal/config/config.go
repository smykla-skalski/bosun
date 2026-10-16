package config

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/disgoorg/snowflake/v2"
)

const (
	DefaultBotHandle = "Bosun"
	DefaultPort      = 8080
)

type Config struct {
	DiscordToken  string
	ApplicationID snowflake.ID
	GuildID       snowflake.ID
	BotHandle     string
	Port          int
	LogLevel      slog.Level
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

	handle := DefaultBotHandle
	if value, ok := lookup("BOT_HANDLE"); ok && strings.TrimSpace(value) != "" {
		handle = strings.TrimSpace(value)
	}

	return Config{
		DiscordToken:  token,
		ApplicationID: applicationID,
		GuildID:       guildID,
		BotHandle:     handle,
		Port:          port,
		LogLevel:      level,
	}, nil
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
