# LLM Providers

OpenPaw is model-agnostic. Providers are registered at startup in `main.go` and
switched at runtime through the `llm.ProviderRegistry`. Any OpenAI-compatible
chat-completions endpoint works via `llm.OpenAICompat`.

| Provider     | Kind (`ProviderType`) | Auth              | Config                      |
|--------------|-----------------------|-------------------|-----------------------------|
| `openrouter` | `openrouter`          | API key           | `OPENROUTER_API_KEY`        |
| `xai`        | `xai`                 | API key           | `XAI_API_KEY`               |
| `xai-oauth`  | `xai-oauth`           | OAuth subscription| `xai_oauth.json` token file |

## xai-oauth — Grok on a SuperGrok / X Premium+ subscription

The `xai-oauth` provider lets a synthetic run inference on a **personal xAI
subscription's limits** (SuperGrok or X Premium+) instead of a metered
`XAI_API_KEY`. It talks to the same OpenAI-compatible endpoint
(`https://api.x.ai/v1/chat/completions`), but the `Authorization: Bearer` is a
short-lived OAuth access token that OpenPaw refreshes automatically.

### How it works

1. `make xai-login` runs an OAuth PKCE flow against `auth.x.ai` and writes a
   token file (default `./xai_oauth.json`) containing the access + refresh
   tokens.
2. On startup, if the token file exists, OpenPaw registers the `xai-oauth`
   provider (`main.go`).
3. Per request, `llm.FileTokenSource.Bearer()` returns the cached access token,
   or — when it is within 5 minutes of expiry — refreshes it against
   `https://auth.x.ai/oauth2/token` and rewrites the file atomically.

### Setup

```bash
# On a machine with a browser (loopback flow):
make xai-login

# On a headless / remote box (device-code flow — enter a code on your phone):
make xai-login ARGS="-device"

# Write somewhere other than ./xai_oauth.json:
make xai-login ARGS="-out data/xai_oauth.json"
```

Then point OpenPaw at the file (if not the default location) and select the
provider:

```bash
# .env
XAI_OAUTH_TOKENS=./xai_oauth.json     # optional; defaults to $CONFIG_DIR/xai_oauth.json
```

Switch the active provider to `xai-oauth` at runtime through the provider
registry (same mechanism as switching to `xai` or `openrouter`).

### Models

A subscription bearer serves a different catalog than a metered `XAI_API_KEY`.
What xAI actually serves a SuperGrok / X Premium+ token:

- **Chat** — `grok-4.5`, `grok-4.3`, `grok-4.20-0309-reasoning`,
  `grok-4.20-0309-non-reasoning`.
- **Media** — `grok-imagine-image` / `-quality`, `grok-imagine-video` / `-1.5`.
- **Nothing else** — no text embeddings, no TTS, no STT. Those cannot move onto
  the subscription at any tier; OpenPaw's memory embeddings stay on Gemini
  (`GEMINI_API_KEY`) whatever the active chat provider is.

Both xAI providers are registered with the same `grokModels` list (`main.go`),
which does not match that catalog:

| Model                                 | `xai` (API key) | `xai-oauth` (subscription) |
|---------------------------------------|-----------------|----------------------------|
| `grok-4.20-0309-reasoning` (default)  | yes             | yes                        |
| `grok-4.20-0309-non-reasoning`        | yes             | yes                        |
| `grok-4-1-fast-non-reasoning`         | yes             | **not served**             |
| `grok-3-mini`                         | yes             | **not served**             |
| `grok-4.5` / `grok-4.3`               | no              | served, but unlisted       |

The default works on both. The two cheap models are advertised but a
subscription bearer 404s them — selecting either under `xai-oauth` fails
upstream. `grok-4.5` and `grok-4.3` are the reverse: served, but
`ProviderRegistry.SetActive` rejects any id absent from `Models`, so add them to
`grokModels` before you can select them.

### Security notes

- The token file holds a long-lived refresh token. It is written mode `0600`;
  keep it out of version control and backups.
- The refresh endpoint is pinned to `x.ai` / `*.x.ai` (`pinXAIEndpoint`): a
  poisoned or misconfigured token endpoint can never receive the refresh token.
- `XAI_OAUTH_CLIENT_ID` / `XAI_OAUTH_TOKEN_URL` override the defaults if you
  hold your own registered OAuth client.

### Caveats

- **Terms of Service.** The default client id impersonates xAI's official Grok
  CLI client (the only id that works for loopback/device logins, mirroring
  Hermes). Automated inference against a personal subscription may violate
  xAI's ToS and risk the account. This is a deliberate operator choice.
- **Tier gating.** xAI restricts OAuth/API inference to certain SuperGrok tiers.
  Even with an active subscription the token endpoint may return **HTTP 403**;
  OpenPaw surfaces this as `TierDeniedError` and re-login will not fix it — fall
  back to an `XAI_API_KEY` (`provider: xai`) instead.
- **Image tools need the API key.** The vision tool (`grok-4-1-fast-non-reasoning`)
  and `generate_image` (`grok-imagine-image`) register only when `XAI_API_KEY` is
  set, and always bill that key — they never use the OAuth bearer. A
  subscription-only install has no image tools at all, and the vision model is one
  a subscription would not serve anyway.
- **Single subscription per process.** `FileTokenSource` refreshes locally under
  an in-process lock. Sharing one subscription across many synths would race on
  the rotating refresh token and can brick the chain. Fleet-wide sharing is
  handled by centralizing the refresh in the hub (see the private snth stack),
  which satisfies the same `llm.CredentialSource` interface.
