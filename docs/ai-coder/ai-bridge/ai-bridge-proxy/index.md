# AI Bridge Proxy

AI Bridge Proxy extends [AI Bridge](../index.md) to support clients that don't allow base URL overrides.
While AI Bridge requires clients to support custom base URLs, many popular AI coding tools lack this capability.

AI Bridge Proxy solves this by acting as an HTTP proxy that intercepts traffic to supported AI providers and forwards it to AI Bridge. Since most clients respect proxy configurations even when they don't support base URL overrides, this provides a universal compatibility layer for AI Bridge.

For a list of clients supported through AI Bridge Proxy, see [Client Configuration](../client-config.md).

## How it works

AI Bridge Proxy operates in two modes depending on the destination:

* MITM (Man-in-the-Middle) mode for allowlisted AI provider domains:
  * Intercepts and decrypts HTTPS traffic using a configured CA certificate
  * Forwards requests to AI Bridge for authentication, auditing, and routing
  * Supports: api.anthropic.com, api.openai.com, api.individual.githubcopilot.com

* Tunnel mode for all other traffic:
  * Passes requests through without decryption

Clients authenticate by passing their Coder token in the proxy credentials.

<!-- TODO(ssncferreira): Add diagram showing how AI Bridge Proxy works in tunnel and MITM modes -->

## When to use AI Bridge Proxy

Use AI Bridge Proxy when your AI tools don't support base URL overrides but do respect standard proxy configurations.

For clients that support base URL configuration, you can use [AI Bridge](../index.md) directly.
Nevertheless, clients with base URL overrides also work with the proxy, in case you want to use multiple AI clients and some of them do not support base URL configuration.

## Next steps

* [Set up AI Bridge Proxy](./setup.md) on your Coder deployment
* [Troubleshoot](./setup.md) common issues
