# Deploy Bosun to smykland

Use this procedure to create one private Discord application and one private Fly Machine.

Version 0.1 uses only static replies. It does not call OpenRouter.

## Prerequisites

- Use a Discord account that can add applications to `smykland`.
- Use a Fly.io account that can create applications and secrets.
- Install the tools with `mise install`.
- Keep all tokens outside the repository.

## Create the Discord application

1. Open the [Discord Developer Portal](https://discord.com/developers/applications).
2. Create an application named `Bosun`.
3. Open the application `Bot` page.
4. Upload `assets/branding/bosun.png` as the bot avatar.
5. Disable `Public Bot` to keep the application private.
6. Enable the `Message Content Intent` option.
7. Copy the bot token to a secure temporary location.
8. Copy the application ID from the `General Information` page.
9. Enable Discord Developer Mode in your Discord client.
10. Copy the `smykland` server ID.

Do not commit the token or place it in `fly.toml`.

## Set the Discord install data

Use a Guild Install for this private application.

Select these OAuth2 scopes:

- `bot`
- `applications.commands`

Select these bot permissions:

- View Channels
- Send Messages
- Send Messages in Threads
- Read Message History

These four permissions use the integer `274877975552`.

Build this install URL with the application ID and the `smykland` server ID:

```text
https://discord.com/oauth2/authorize?client_id=<DISCORD_APPLICATION_ID>&integration_type=0&scope=bot%20applications.commands&permissions=274877975552&guild_id=<DISCORD_GUILD_ID>&disable_guild_select=true
```

## Install Bosun in smykland

Open the completed URL. Approve the private install for `smykland` before the first Fly deploy.

Bosun registers its guild command when it starts. Discord must authorize Bosun in the guild before that request.

## Prepare Fly.io

Authenticate the Fly CLI:

```sh
fly auth login
```

Use the default Fly application name:

```sh
export FLY_APP_NAME="smykland-bosun"
fly apps create "$FLY_APP_NAME"
```

Use the Discord application ID suffix if the default name already exists:

```sh
export DISCORD_APPLICATION_ID="<application-id>"
export FLY_APP_NAME="smykland-bosun-${DISCORD_APPLICATION_ID}"
fly apps create "$FLY_APP_NAME"
```

Keep this variable in the shell for all later commands.

The `app` value in `fly.toml` remains the default name. Each Fly task uses `FLY_APP_NAME` as an explicit override.

Validate the configuration:

```sh
mise run fly:validate
```

The configuration uses one `shared-cpu-1x` Machine with 256 MB in `fra`.

The configuration has no public service. Its private readiness check calls `/readyz` on port 8080.

The `always` restart policy keeps the Discord gateway process active.

## Set the Fly secrets

Run this command, paste the three lines, and press Control-D:

```sh
fly secrets import --app "$FLY_APP_NAME"
```

```text
DISCORD_TOKEN=<bot-token>
DISCORD_APPLICATION_ID=<application-id>
DISCORD_GUILD_ID=<smykland-server-id>
```

Fly stores these values as application secrets. Bosun does not write them to logs.

## Validate and deploy

Run the full local gate:

```sh
mise run check
```

Build the same container locally:

```sh
mise run docker:build
```

Deploy one Machine:

```sh
mise run fly:deploy
```

The deploy command blocks new public IP addresses and keeps one Machine in `fra`.

## Verify the release

Confirm that the Fly Machine and its readiness check are healthy:

```sh
fly status --app "$FLY_APP_NAME"
fly checks list --app "$FLY_APP_NAME"
fly logs --app "$FLY_APP_NAME"
```

Test these actions in a `smykland` text channel:

- Mention `@Bosun`.
- Reply to a Bosun message.
- Write `Bosun` as a separate word.
- Run `/ask question:test`.

Confirm that each action gets one Polish reply. Confirm that direct messages and other servers get no reply.

## Stop or replace the release

Stop the only Machine when the bot must stay offline:

```sh
fly scale count 0 --app "$FLY_APP_NAME" --yes
```

List the release images and select the prior good image:

```sh
fly releases --app "$FLY_APP_NAME" --image
```

Deploy that prior image:

```sh
fly deploy --app "$FLY_APP_NAME" --config fly.toml --image <previous-image-reference> --ha=false --no-public-ips
```

## Next release

Add OpenRouter model calls in the next release.

Add the balance check, zero-credit fallback, and retry cooldown before those calls become active.

## Official references

- [Discord OAuth2](https://discord.com/developers/docs/topics/oauth2)
- [Discord gateway intents](https://discord.com/developers/docs/events/gateway#gateway-intents)
- [Discord permissions](https://discord.com/developers/docs/topics/permissions)
- [Discord application commands](https://discord.com/developers/docs/interactions/application-commands)
- [Fly application configuration](https://fly.io/docs/reference/configuration/)
- [Fly Machine restart policy](https://fly.io/docs/machines/guides-examples/machine-restart-policy/)
