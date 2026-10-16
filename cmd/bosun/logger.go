package main

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/disgoorg/disgo/events"
	"github.com/disgoorg/snowflake/v2"
)

type discardHandler struct{}

func (discardHandler) Enabled(context.Context, slog.Level) bool {
	return false
}

func (discardHandler) Handle(context.Context, slog.Record) error {
	return nil
}

func (h discardHandler) WithAttrs([]slog.Attr) slog.Handler {
	return h
}

func (h discardHandler) WithGroup(string) slog.Handler {
	return h
}

func dependencyLogger(*slog.Logger) *slog.Logger {
	return slog.New(discardHandler{})
}

func safeErrorType(err error) string {
	if err == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%T", err)
}

func gatewayReadyHandler(logger *slog.Logger, guildID snowflake.ID) func(*events.Ready) {
	return func(event *events.Ready) {
		logGatewayReady(logger, guildID, event.ShardID(), "identified")
	}
}

func gatewayResumedHandler(logger *slog.Logger, guildID snowflake.ID) func(*events.Resumed) {
	return func(event *events.Resumed) {
		logGatewayReady(logger, guildID, event.ShardID(), "resumed")
	}
}

func logGatewayReady(logger *slog.Logger, guildID snowflake.ID, shardID int, transition string) {
	logger.Info("discord gateway state transition",
		"gateway_state", "ready",
		"transition", transition,
		"guild_id", guildID,
		"shard_id", shardID,
	)
}

func logGatewayDisconnected(logger *slog.Logger, guildID snowflake.ID, terminal gatewayTermination) {
	logger.Error("discord gateway state transition",
		"gateway_state", "disconnected",
		"transition", "terminal_close",
		"guild_id", guildID,
		"shard_id", terminal.shardID,
		"error_type", terminal.errorType,
	)
}
