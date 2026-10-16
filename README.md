# Bosun

Bosun is a private Discord bot for the `smykland` server.

Version 0.1 selects Polish jokes from 1,000 static replies. It does not call OpenRouter.

Bosun replies to these events:

- A member mentions `@Bosun`.
- A member replies to a Bosun message.
- A member writes the separate word `Bosun`.
- A member uses `/ask question:<text>`.

Bosun ignores direct messages, other servers, bots, webhooks, and its own messages.

## Requirements

- Install [mise](https://mise.jdx.dev/).
- Install Docker for a local container build.
- Install [flyctl](https://fly.io/docs/flyctl/install/) for a Fly deploy.
- Create the private Discord application from [the deployment guide](docs/deployment.md).

## Local validation

Install the pinned Go toolchain:

```sh
mise install
```

Run all local checks:

```sh
mise run check
```

Build the container:

```sh
mise run docker:build
```

## Local run

Set these required variables in the process environment:

```text
DISCORD_TOKEN=<bot-token>
DISCORD_APPLICATION_ID=<application-id>
DISCORD_GUILD_ID=<smykland-server-id>
```

Use these optional variables when the defaults do not apply:

| Variable | Default | Purpose |
| --- | --- | --- |
| `BOT_HANDLE` | `Bosun` | Set the plain text handle. |
| `PORT` | `8080` | Set the health server port. |
| `LOG_LEVEL` | `info` | Set `debug`, `info`, `warn`, or `error`. |

Build and start Bosun:

```sh
mise run build
./build/bosun
```

Check the local live endpoint:

```sh
curl --fail http://127.0.0.1:8080/livez
```

The `/readyz` endpoint returns success after the Discord gateway becomes ready.

## Deployment

Use [docs/deployment.md](docs/deployment.md) to create, deploy, install, and test Bosun.

The Fly tasks use `smykland-bosun` unless `FLY_APP_NAME` selects the documented fallback name.

OpenRouter support and its credit control belong to the next release.

## Main tasks

List all tasks with `mise tasks ls`.

| Task | Result |
| --- | --- |
| `mise run check` | Run the format, catalog, lint, test, race, and build checks. |
| `mise run replies:generate` | Create the 1,000 reply catalog. |
| `mise run docker:build` | Build `smykland-bosun:local`. |
| `mise run fly:validate` | Validate `fly.toml` with strict checks. |
| `mise run fly:deploy` | Deploy one private Fly Machine in Frankfurt. |
