---
title: OpenTelemetry tracing
---

Coder Agents exports OpenTelemetry traces that follow the [OpenTelemetry GenAI semantic conventions](https://opentelemetry.io/docs/specs/semconv/gen-ai/).
Observability backends that understand these conventions can attribute model usage, token spend, and tool activity to each user without custom mapping.

The GenAI conventions are still in development.
Coder Agents follows the span names and `gen_ai.*` attributes in semantic conventions v1.41.0, and they might change as the conventions stabilize.

## Enable tracing

Start the Coder server with [`--trace`](../../reference/cli/server/index.md#--trace) (`CODER_TRACE_ENABLE=true`).
Coder Agents spans are exported with the rest of the server's traces.

The server exports traces with OTLP over gRPC.
Set the collector address with `OTEL_EXPORTER_OTLP_ENDPOINT` or `OTEL_EXPORTER_OTLP_TRACES_ENDPOINT`, and add headers with `OTEL_EXPORTER_OTLP_HEADERS`.
`OTEL_EXPORTER_OTLP_PROTOCOL` has no effect, so OTLP over HTTP is not supported.

The connection is unencrypted by default, even for an `https://` endpoint or with `OTEL_EXPORTER_OTLP_INSECURE=false`.
To use TLS, set `OTEL_EXPORTER_OTLP_CERTIFICATE` to the collector's CA certificate, and set `OTEL_EXPORTER_OTLP_CLIENT_CERTIFICATE` and `OTEL_EXPORTER_OTLP_CLIENT_KEY` for mutual TLS.

## Spans

Each chat turn is a separate trace.
The root span covers the turn, and the model calls and tool calls of the turn are its descendants, including advisor and compaction calls.
Model calls that run after a turn, such as title, status label, and chat summary generation, are traces of their own.
They carry the same chat and user attributes, so you can attribute their tokens to the same user.

| Span name                         | `gen_ai.operation.name` | Covers                                      |
|-----------------------------------|-------------------------|---------------------------------------------|
| `invoke_agent coder`              | `invoke_agent`          | One chat turn, from the prompt to the reply |
| `chat {gen_ai.request.model}`     | `chat`                  | One model call, with span kind `CLIENT`     |
| `execute_tool {gen_ai.tool.name}` | `execute_tool`          | One tool call that Coder Agents runs        |

The turn also contains spans for other lifecycle stages, such as `generation_step`, `prepare`, `commit`, and `compaction`.
Every span carries a `stage` attribute with its lifecycle stage name, such as `chat_turn`, `stream`, or `tool_call`.
Model calls outside the assistant stream, such as compaction and title generation, have the stage `model_call`.
The `stage` attribute matches the `stage` label of the `coderd_chatd_stage_duration_seconds` Prometheus metric.

## Attributes

All three GenAI spans carry attributes that identify the chat and its owner:

| Attribute                | Value                                         |
|--------------------------|-----------------------------------------------|
| `gen_ai.conversation.id` | The chat ID                                   |
| `user.id`                | The ID of the user who owns the chat          |
| `coder.organization.id`  | The ID of the chat's organization             |
| `coder.workspace.id`     | The ID of the chat's workspace, if it has one |
| `coder.chat.parent_id`   | The ID of the parent chat, for subagent chats |
| `coder.chat.root_id`     | The ID of the root chat, for subagent chats   |

The `invoke_agent coder` span also carries `gen_ai.agent.name` (`coder`) and, once the turn selects a model, `gen_ai.provider.name`.

The `chat` span carries the model call's details:

| Attribute                                  | Value                                                         |
|--------------------------------------------|---------------------------------------------------------------|
| `gen_ai.provider.name`                     | The provider, such as `openai`, `anthropic`, or `aws.bedrock` |
| `gen_ai.request.model`                     | The model name                                                |
| `gen_ai.request.max_tokens`                | The output token limit Coder Agents requested, if one is set  |
| `gen_ai.request.temperature`               | The temperature Coder Agents requested, if one is set         |
| `gen_ai.request.stream`                    | Whether the response was streamed                             |
| `gen_ai.response.id`                       | The response ID the provider reported, if any                 |
| `gen_ai.response.finish_reasons`           | Why the model stopped, if the provider reported it            |
| `gen_ai.usage.input_tokens`                | Input tokens, including cache reads and cache writes          |
| `gen_ai.usage.output_tokens`               | Output tokens, including reasoning tokens                     |
| `gen_ai.usage.cache_read.input_tokens`     | Input tokens read from the provider's prompt cache, if any    |
| `gen_ai.usage.cache_creation.input_tokens` | Input tokens written to the provider's prompt cache, if any   |
| `gen_ai.usage.reasoning.output_tokens`     | Reasoning tokens, if the provider reports them                |
| `gen_ai.output.type`                       | `json`, for structured output requested in OpenAI's JSON mode |

The request attributes describe the limits Coder Agents asks for, not always the request sent to the provider.
For example, a model configuration can set an OpenAI-specific output token limit that replaces the general limit, and some providers ignore temperature for reasoning models.

### Token usage limits

Token counts come from the provider's response, and some providers report less than they bill:

- **Anthropic:** when a response runs server-side tools such as web search, the model works in several internal iterations.
  `gen_ai.usage.input_tokens` and the cache attributes report the input of the first iteration only, so they undercount the billed input.
  Output tokens cover the whole response.
- **OpenAI-compatible providers, such as OpenRouter:** cache writes are not reported, so `gen_ai.usage.cache_creation.input_tokens` is absent.
  The input token total still includes them.

For spend, use [AI Governance cost control](../../ai-governance/ai-gateway/cost-controls.md), which records the usage AI Gateway receives from the provider, rather than summing span tokens.

The `execute_tool` span carries `gen_ai.tool.name`, `gen_ai.tool.call.id`, and `gen_ai.tool.type`.
The tool type is `function` for built-in Coder Agents tools and `extension` for other tools, such as tools from MCP servers.

On `chat` and `execute_tool` spans, the operation name, provider, model, and tool attributes are set when the span starts, so samplers can use them.
The `invoke_agent coder` span starts before the turn selects a model, so samplers see its operation name and chat attributes but not `gen_ai.provider.name`.
A failed span has an error status and an `error.type` attribute.
A tool call that returns an error result, such as an MCP server that cannot be reached, ends with `error.type` set to `tool_error`.

## Recorded content

GenAI attributes contain metadata only.
Prompts, model responses, and tool arguments and results are not recorded as attributes.

When tracing is enabled, the server also attaches its log messages to the active span as span events.
Some of those messages contain content, such as the error text a tool returns to the model.
If your tracing backend must not hold that content, drop span events whose name starts with `log:` in your collector.
