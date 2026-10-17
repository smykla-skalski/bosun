# Deploy Bosun to configured-server

Use this procedure to create one private Discord application and one private Fly Machine.

Version 0.3 supports static and dynamic replies. Static replies remain the guaranteed fallback.

## Prerequisites

- Use a Discord account that can add applications to `configured-server`.
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
10. Copy the `configured-server` server ID.

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

Build this install URL with the application ID and the `configured-server` server ID:

```text
https://discord.com/oauth2/authorize?client_id=<DISCORD_APPLICATION_ID>&integration_type=0&scope=bot%20applications.commands&permissions=274877975552&guild_id=<DISCORD_GUILD_ID>&disable_guild_select=true
```

## Install Bosun in configured-server

Open the completed URL. Approve the private install for `configured-server` before the first Fly deploy.

Bosun registers its guild command when it starts. Discord must authorize Bosun in the guild before that request.

## Prepare Fly.io

Authenticate the Fly CLI:

```sh
fly auth login
```

Use the default Fly application name:

```sh
export FLY_APP_NAME="bosun-discord-bot"
fly apps create "$FLY_APP_NAME"
```

Use the Discord application ID suffix if the default name already exists:

```sh
export DISCORD_APPLICATION_ID="<application-id>"
export FLY_APP_NAME="bosun-discord-bot-${DISCORD_APPLICATION_ID}"
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

## Select the reply mode

Leave `OPENROUTER_API_KEY` unset to use only the 6,400 static replies.

Set `OPENROUTER_API_KEY` to enable dynamic replies. Raw member text stays inside the Bosun process.

Bosun creates a local profile from fixed labels. The labels describe the conversation role, question kind, response mode, form, structure, tone, and spice.

Bosun does not send member words, names, links, files, identifiers, timestamps, references, exact counts, or stable hashes.

Bosun selects eight vetted static replies for each event. Candidate zero is the guaranteed fallback.

Each candidate batch has distinct lead and finisher variants. Consecutive replies do not reuse either variant.

Each reply is one dash-linked sentence without final punctuation. Its lead answers the exact type, and its finisher adds a related reversal.

OpenRouter receives the fixed profile and eight typed candidate texts. Jev returns one typed choice, eight probabilities, and a confidence value.

Bosun displays the selected local candidate. It never displays text from the model response.

Bosun records only the committed candidate in the recent reply history. It releases an unused batch without a history record.

Each decision requires `provider.zdr=true`, `provider.data_collection="deny"`, and `provider.require_parameters=true`. Each decision also sends `X-OpenRouter-Cache: false`.

Bosun does not retry with weaker privacy controls. A privacy route fault returns the preselected static reply.

OpenRouter still sees the API key, source address, request time, model, fixed labels, candidate texts, token data, cost, and workspace link.

These controls do not give full anonymity. Leave `OPENROUTER_API_KEY` unset for full separation from OpenRouter and its model providers.

Use these optional settings when the defaults do not apply:

| Variable | Default | Purpose |
| --- | --- | --- |
| `OPENROUTER_MODEL` | `typesafe/jev-1.14` | Use the only decision model that the code permits. |
| `OPENROUTER_BALANCE_COOLDOWN` | `15m` | Set the credit circuit cooldown. |
| `OPENROUTER_REQUEST_TIMEOUT` | `5s` | Set the request deadline. |

## Set the Fly secrets

Run this command, paste the three lines, and press Control-D:

```sh
fly secrets import --app "$FLY_APP_NAME"
```

```text
DISCORD_TOKEN=<bot-token>
DISCORD_APPLICATION_ID=<application-id>
DISCORD_GUILD_ID=<configured-server-server-id>
```

Run this command to enable dynamic mode:

```sh
fly secrets import --app "$FLY_APP_NAME"
```

Paste this line with the normal inference key. Then press Control-D:

```text
OPENROUTER_API_KEY=<normal-inference-key>
```

Do not use an OpenRouter management key. The normal inference key is sufficient.

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

## Enable automatic deployment

Create one app-scoped Fly deploy token for the `bosun-discord-bot` application.

Store that token as the `FLY_API_TOKEN` repository secret.

Each push to `main` runs `mise run check`, verifies the required Fly secrets, and deploys one Machine.

The workflow rejects multiple Machines before deployment. It verifies one started Machine and all health checks after deployment.

The production token does not need access to another Fly application.

## Verify the release

Confirm that the Fly Machine and its readiness check are healthy:

```sh
fly status --app "$FLY_APP_NAME"
fly checks list --app "$FLY_APP_NAME"
fly logs --app "$FLY_APP_NAME"
```

Test these actions in a `configured-server` text channel:

- Mention `@Bosun`.
- Reply to a Bosun message.
- Write `Bosun` as a separate word.
- Run `/ask question:test`.

Confirm that each action gets one Polish reply. Confirm that direct messages and other servers get no reply.

In dynamic mode, confirm that a successful reply log has `source=openrouter`. Do not expect the log to contain member or model text.

## Operate the credit circuit

The normal `/api/v1/key` endpoint shows the current inference key limit. It does not show the complete account balance.

Bosun treats the final OpenRouter `402` response as the account credit source of truth. A structured `403` budget fault has the same result.

Bosun makes no OpenRouter request while this long circuit is open. Readiness stays healthy because it depends only on the Discord gateway.

After the cooldown, exactly one request checks the normal key limit. Concurrent requests use static replies through the recovery probe.

A positive remaining key limit or an explicit `limit_remaining:null` permits one shared decision probe.

Only a successful decision probe closes the circuit. Any failed probe restarts the long cooldown, except `openrouter_in_flight_budget`.

Missing key data, null key data, or a missing limit restarts the long cooldown.

The `openrouter_in_flight_budget` source is temporary. This rule also applies during each recovery probe.

Bosun keeps the recovery state and uses only the bounded temporary delay. After this delay, one request starts a shared recovery attempt.

During normal closed-circuit operation, Bosun honors `Retry-After` locally for `429` and `503` responses. These responses do not open the long circuit.

After a recovery key check returns `429` or `503` without a valid positive `Retry-After`, Bosun waits five seconds. Bosun honors a valid longer value up to 15 minutes.

Bosun does not retry OpenRouter within one Discord request. Any remote fault returns the selected static reply.

## Verify the privacy route

Confirm that the release uses version 0.3 or later before you enable dynamic mode.

The automated request test checks the complete JSON body and headers. It uses canary names, identifiers, links, service names, and file names.

The test permits only fixed profile keys, one typed choice question, and eight vetted candidate criteria.

The test also checks the ZDR, data collection, required parameter, and cache controls.

Do not use a live member message to inspect the request body. Use the local request test with its fake endpoint.

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

## Official references

- [Discord OAuth2](https://discord.com/developers/docs/topics/oauth2)
- [Discord gateway intents](https://discord.com/developers/docs/events/gateway#gateway-intents)
- [Discord permissions](https://discord.com/developers/docs/topics/permissions)
- [Discord application commands](https://discord.com/developers/docs/interactions/application-commands)
- [Fly application configuration](https://fly.io/docs/reference/configuration/)
- [Fly Machine restart policy](https://fly.io/docs/machines/guides-examples/machine-restart-policy/)
- [OpenRouter Jev 1.14](https://openrouter.ai/typesafe/jev-1.14)
- [OpenRouter Decisions API guide](https://openrouter.ai/blog/insights/what-is-jev/)
- [OpenRouter current key](https://openrouter.ai/docs/api/api-reference/api-keys/get-current-key)
- [OpenRouter privacy policy](https://openrouter.ai/privacy/)
- [OpenRouter data collection](https://openrouter.ai/docs/guides/privacy/data-collection)
- [OpenRouter Zero Data Retention](https://openrouter.ai/docs/guides/features/zdr)
- [OpenRouter provider routing](https://openrouter.ai/docs/guides/routing/provider-selection)
- [OpenRouter response cache](https://openrouter.ai/docs/guides/features/response-caching)
