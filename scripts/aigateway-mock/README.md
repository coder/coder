# AI Gateway mock upstream

Record the headers AI Gateway sends upstream without calling a real AI provider.
The server replays the repository's embedded response fixtures for OpenAI Chat Completions, OpenAI Responses, and Anthropic Messages.
It needs no provider credentials, database, or running Coder instance to start.

## Run

From the repository root:

```sh
go run ./scripts/aigateway-mock
```

The server listens on `http://127.0.0.1:18081` and prints its address to stderr.
Each accepted request produces one JSON line on stdout.
Use `--listen 127.0.0.1:18082` to choose another address, or port `0` for an available port.
Stop the process with Ctrl+C.

To build an executable that runs from any directory:

```sh
go build -o /tmp/aigateway-mock ./scripts/aigateway-mock
/tmp/aigateway-mock
```

Fixtures are embedded in the executable; there are no runtime fixture-file dependencies.

## Inspect a request

```sh
curl --fail-with-body http://127.0.0.1:18081/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -H 'X-Smoke-ID: test-user' \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}],"stream":false}'
```

The response is a fixed Chat Completions fixture.
The server writes this record to stdout:

```json
{"path":"/v1/chat/completions","stream":false,"headers":{"X-Smoke-Id":["test-user"]}}
```

Header values remain arrays so duplicate identity headers are visible.
By default, only headers beginning with `X-AI-Bridge-Actor-` or `X-Smoke-` are recorded, case-insensitively.
To inspect other configured header names, repeat `--header`:

```sh
go run ./scripts/aigateway-mock --header X-User-ID --header X-User-Email
```

Request bodies, query strings, authorization headers, and cookies are not recorded by default.
Actor headers can contain personal information, and explicitly selected headers are logged verbatim.
Use test accounts and fake provider keys, protect any captured logs, and do not expose this unauthenticated server publicly.

## Connect AI Gateway

In the Coder dashboard, open **Admin settings** > **AI** > **Providers** and add a temporary provider:

| Type      | Example name      | Base URL                     | API key      |
|-----------|-------------------|------------------------------|--------------|
| OpenAI    | `smoke-openai`    | `http://127.0.0.1:18081/v1/` | `smoke-only` |
| Anthropic | `smoke-anthropic` | `http://127.0.0.1:18081/`    | `smoke-only` |

The loopback URLs assume the gateway and mock share a network namespace.
If they run in different containers or machines, use an address reachable from the gateway and restrict network access to the mock.

Enable `CODER_AI_GATEWAY_SEND_ACTOR_HEADERS=true` on the gateway process, then send requests through the provider's route:

```sh
curl --fail-with-body "$CODER_URL/api/v2/ai-gateway/smoke-openai/v1/chat/completions" \
  -H "Authorization: Bearer $CODER_TOKEN" \
  -H 'Content-Type: application/json' \
  -H 'X-AI-Bridge-Actor-ID: forged-id' \
  -H 'X-Smoke-Trace: preserve-me' \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Hello"}],"stream":false}'
```

Set `CODER_URL` to your Coder API URL and `CODER_TOKEN` to a test user's token.
The recorded actor ID should identify that user, not `forged-id`, and `X-Smoke-Trace` should remain `preserve-me`.
Remove the temporary provider when finished.

## Supported requests and limits

The server accepts `POST` requests to `/v1/chat/completions`, `/v1/responses`, and `/v1/messages`, with equivalent routes without `/v1`.
A JSON object with `"stream": true` selects the SSE fixture; omitting `stream` or setting it to `false` selects JSON.
For example, use `{"input":"Hello","stream":true}` for Responses, or `{"messages":[{"role":"user","content":"Hello"}],"max_tokens":32}` for Messages.
Add `curl -N` when inspecting SSE output.

This is a header-inspection tool, not a full provider emulator.
It does not authenticate requests, validate models or prompts, or generate responses based on input.
Response IDs, models, timestamps, text, and usage come from fixed fixtures.
SSE fixtures are written immediately without simulated token delays.
Unknown routes return `404`, unsupported methods return `405`, malformed JSON returns `400`, and request bodies larger than 1 MiB return `413`.
Passthrough endpoints such as `/v1/models` and provider error simulation are not supported.

## Test

```sh
go test -race ./scripts/aigateway-mock
```
