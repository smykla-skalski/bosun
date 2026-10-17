# Bosun

Bosun is a private Discord bot for the `configured-server` server.

Version 0.3 first classifies each message inside the process. It then selects one of 4,000 Polish static replies by category.

Bosun starts with the `harsh` spice level. It selects `savage` after the third trigger in ten minutes.

Bosun uses the selected static reply for every model fault. This fallback includes timeouts, overload, invalid output, and unavailable credit.

In dynamic mode, Bosun selects eight vetted local replies. OpenRouter chooses one fixed index and never creates the displayed text.

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
DISCORD_GUILD_ID=<configured-server-server-id>
```

Use these optional variables when the defaults do not apply:

| Variable | Default | Purpose |
| --- | --- | --- |
| `BOT_HANDLE` | `Bosun` | Set the plain text handle. |
| `PORT` | `8080` | Set the health server port. |
| `LOG_LEVEL` | `info` | Set `debug`, `info`, `warn`, or `error`. |
| `OPENROUTER_API_KEY` | unset | Enable dynamic replies. Keep static mode when this value is unset. |
| `OPENROUTER_MODEL` | `google/gemini-3.1-flash-lite` | Use the only model that the code permits. |
| `OPENROUTER_BALANCE_COOLDOWN` | `15m` | Set the wait period after a final credit error. |
| `OPENROUTER_REQUEST_TIMEOUT` | `5s` | Set one OpenRouter request deadline. |

Dynamic mode never sends raw or modified member text to OpenRouter. Raw member text stays inside the Bosun process.

Bosun sends fixed profile labels and eight vetted static reply candidates. Each candidate has a fixed index from zero through seven.

OpenRouter returns only one index. Bosun displays the selected local candidate and never displays model text.

Bosun reserves each candidate batch. It adds only the committed reply to the recent reply history.

Each candidate batch has eight distinct setup variants and eight distinct punchline variants.

Each reply uses two sentence roles. The first sentence fits the message, and the second adds a separate comic scene.

The OpenRouter request requires Zero Data Retention, denies data collection, and requires JSON Schema support. It also disables the response cache.

OpenRouter still sees the API key, source address, request time, model, fixed labels, candidate texts, token data, cost, and workspace link.

These controls do not give full anonymity. Use static mode for full separation from OpenRouter and its model providers.

Remove `OPENROUTER_API_KEY` to select static mode.

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

The Fly tasks use `bosun-discord-bot` unless `FLY_APP_NAME` selects the documented fallback name.

A push to `main` runs every local check and deploys one healthy Fly Machine through GitHub Actions.

The normal OpenRouter key endpoint reports only the inference key limit. It does not report the complete account balance.

Bosun treats a final `402` response as the account credit source of truth. A structured `403` budget fault also opens the credit circuit.

Bosun makes no OpenRouter calls during the credit cooldown. During normal closed-circuit operation, it honors `Retry-After` locally for temporary `429` and `503` responses.

After a recovery key check returns `429` or `503` without a valid positive `Retry-After`, Bosun waits five seconds. Bosun limits all `Retry-After` delays to 15 minutes.

After the cooldown, one request checks the key limit. Other concurrent requests use static replies through the recovery probe.

A positive key limit or an explicit `limit_remaining:null` permits one shared completion probe. Only a successful probe closes the circuit.

Missing key data, null key data, or a missing limit restarts the long cooldown.

Any failed probe restarts the long cooldown, except `openrouter_in_flight_budget`. This source keeps recovery active and uses only its temporary bounded delay.

## OpenRouter privacy controls

Bosun validates all profile labels and candidates before network access. An invalid value returns candidate zero.

The completion body contains the model, two fixed-role messages, eight indexed candidates, a strict choice schema, token controls, and provider policy.

OpenRouter must return exactly one JSON object with one integer choice from zero through seven. Any other output returns candidate zero.

Bosun omits user IDs, session IDs, metadata, traces, attribution headers, tools, and plugins.

OpenRouter can still connect a request to its API key and workspace. Its privacy policy also permits source address and service data collection.

Zero Data Retention stops endpoint retention. It does not stop request transfer to the selected model provider.

See the official [privacy policy](https://openrouter.ai/privacy/), [ZDR guide](https://openrouter.ai/docs/guides/features/zdr), [provider routing guide](https://openrouter.ai/docs/guides/routing/provider-selection), [structured output guide](https://openrouter.ai/docs/guides/features/structured-outputs), and [response cache guide](https://openrouter.ai/docs/guides/features/response-caching).

## Main tasks

List all tasks with `mise tasks ls`.

| Task | Result |
| --- | --- |
| `mise run check` | Run the format, catalog, lint, test, race, and build checks. |
| `mise run replies:generate` | Create the 4,000 reply catalog. |
| `mise run workflow:check` | Check the GitHub Actions workflow. |
| `mise run docker:build` | Build `bosun-discord-bot:local`. |
| `mise run fly:validate` | Validate `fly.toml` with strict checks. |
| `mise run fly:deploy` | Deploy one private Fly Machine in Frankfurt. |
