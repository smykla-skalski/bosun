package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/disgoorg/disgo"
	"github.com/disgoorg/disgo/bot"
	"github.com/disgoorg/disgo/gateway"
	"github.com/disgoorg/disgo/rest"
	"github.com/disgoorg/snowflake/v2"

	botruntime "github.com/smykla-skalski/bosun/internal/bot"
	"github.com/smykla-skalski/bosun/internal/config"
	"github.com/smykla-skalski/bosun/internal/health"
	"github.com/smykla-skalski/bosun/internal/replies"
)

const shutdownTimeout = 10 * time.Second

var errGatewayTerminated = errors.New("Discord gateway closed permanently")

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("configuration is invalid", "error", err)
		os.Exit(1)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel}))
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err = run(ctx, cfg, logger); err != nil {
		logger.Error("bosun stopped", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	catalog, err := replies.LoadEmbedded()
	if err != nil {
		return fmt.Errorf("load reply catalog: %w", err)
	}
	service, err := botruntime.NewService(
		cfg.GuildID,
		cfg.ApplicationID,
		cfg.BotHandle,
		replies.NewSelector(catalog),
		logger,
	)
	if err != nil {
		return fmt.Errorf("build bot service: %w", err)
	}

	gatewayTerminated := make(chan struct{}, 1)
	client, err := newDiscordClient(cfg.DiscordToken, service, dependencyLogger(logger), gatewayTerminated)
	if err != nil {
		closeService(service, logger)
		return fmt.Errorf("build Discord client: %w", err)
	}
	if client.ApplicationID != cfg.ApplicationID {
		closeClientAndService(client, service, logger)
		return errors.New("DISCORD_APPLICATION_ID does not match DISCORD_TOKEN")
	}

	healthServer, err := health.New(cfg.Address(), func() bool {
		return client.Gateway != nil && client.Gateway.Status() == gateway.StatusReady
	})
	if err != nil {
		closeClientAndService(client, service, logger)
		return fmt.Errorf("build health server: %w", err)
	}
	defer closeResources(client, service, healthServer, logger)

	if err = registerGuildCommands(ctx, client.Rest, cfg.ApplicationID, cfg.GuildID); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("register guild commands: %w", err)
	}

	healthErrors := make(chan error, 1)
	go func() {
		healthErrors <- healthServer.ListenAndServe()
	}()
	if err = client.OpenGateway(ctx); err != nil {
		if ctx.Err() != nil {
			return nil
		}
		return fmt.Errorf("open Discord gateway: %w", err)
	}
	logger.Info("bosun is ready", "guild_id", cfg.GuildID)

	return waitForExit(ctx, healthErrors, gatewayTerminated)
}

func registerGuildCommands(ctx context.Context, applications rest.Applications, applicationID snowflake.ID, guildID snowflake.ID) error {
	_, err := applications.SetGuildCommands(applicationID, guildID, botruntime.Commands(), rest.WithCtx(ctx))
	return err
}

func newDiscordClient(token string, service *botruntime.Service, logger *slog.Logger, terminated chan<- struct{}) (*bot.Client, error) {
	listeners := []bot.EventListener{
		bot.NewListenerFunc(service.OnMessageCreate),
		bot.NewListenerFunc(service.OnApplicationCommand),
	}
	return disgo.New(token,
		bot.WithLogger(logger),
		bot.WithEventManagerConfigOpts(eventManagerConfigOpts(listeners...)...),
		bot.WithGatewayConfigOpts(
			gateway.WithAutoReconnect(true),
			gateway.WithCloseHandler(gatewayCloseHandler(terminated)),
			gateway.WithIntents(
				gateway.IntentGuildMessages,
				gateway.IntentMessageContent,
			),
		),
	)
}

func eventManagerConfigOpts(listeners ...bot.EventListener) []bot.EventManagerConfigOpt {
	return []bot.EventManagerConfigOpt{
		bot.WithAsyncEventsEnabled(),
		bot.WithListeners(listeners...),
	}
}

func gatewayCloseHandler(terminated chan<- struct{}) gateway.CloseHandlerFunc {
	return func(gateway.Gateway, error, bool) {
		select {
		case terminated <- struct{}{}:
		default:
		}
	}
}

func waitForExit(ctx context.Context, healthErrors <-chan error, gatewayTerminated <-chan struct{}) error {
	select {
	case <-ctx.Done():
		return nil
	case err := <-healthErrors:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve health endpoints: %w", err)
	case <-gatewayTerminated:
		return errGatewayTerminated
	}
}

func closeResources(client *bot.Client, service *botruntime.Service, healthServer *health.Server, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := service.Close(ctx); err != nil {
		logger.Error("bot service shutdown failed", "error_type", fmt.Sprintf("%T", err))
	}
	client.Close(ctx)
	if err := healthServer.Shutdown(ctx); err != nil {
		logger.Error("health server shutdown failed", "error_type", fmt.Sprintf("%T", err))
	}
}

func closeClientAndService(client *bot.Client, service *botruntime.Service, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := service.Close(ctx); err != nil {
		logger.Error("bot service shutdown failed", "error_type", fmt.Sprintf("%T", err))
	}
	client.Close(ctx)
}

func closeService(service *botruntime.Service, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := service.Close(ctx); err != nil {
		logger.Error("bot service shutdown failed", "error_type", fmt.Sprintf("%T", err))
	}
}
