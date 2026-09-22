# Header command refresh

**Linear:** TBD

**Related issue:** [coder/coder#23889](https://github.com/coder/coder/issues/23889)

**Eng owner:** TBD · **Reviewer:** TBD

## 🎯 Intent

We currently execute header commands once when we [create the HTTP transport](https://github.com/coder/coder/blob/main/cli/root.go#L1939). Long-running processes like `coder ssh`, `coder agent`, and `coder wsproxy` keep using that output even after the token expires. We will cache the output until 10 seconds before the earliest JWT expiration and rerun the command on the next HTTP request.

## 🧩 How it works

We will give `codersdk.HeaderTransport` a header provider. Static headers and command-generated headers will implement the same interface. The command provider caches headers and reruns the command after the cache expires. We won't change the command's output format.

There is no TTL flag. We inspect bare JWT values and values prefixed with `Bearer` for an `exp` claim. If no value has a usable expiration, the output stays cached for the life of the provider.

### Implementation notes

Add this interface in `codersdk`:

```go
type HeaderProvider interface {
    Headers(context.Context) (http.Header, error)
}
```

- Replace [`HeaderTransport.Header`](https://github.com/coder/coder/blob/9d9733727748fdeaa8b7f73a6e33f1cee192bc8a/codersdk/client.go#L879-L907) with `Provider HeaderProvider`. `RoundTrip` always calls `Provider.Headers`; there is no static/dynamic switch. Static callers use `StaticHeaderProvider`, which returns a copy of its configured headers. Use an empty static provider when no headers are configured.
- In `cli/header.go`, move command execution and parsing into `commandHeaderProvider`. On refresh, combine command output with static headers from `--header` or `--agent-header` and replace the complete cached map. `headerTransport` selects the provider at construction for root, agent, and wsproxy clients.
- Call `Headers` during construction to preserve startup validation and populate the cache. Replace direct reads of `HeaderTransport.Header`, including DERP setup, with calls to `Provider.Headers(ctx)`. DERP still receives a fixed snapshot, not a refresh callback.
- In `RoundTrip`, keep adding headers to the provided request as today; request copying is outside this change. If a refresh fails, return the error without sending the request, reusing expired headers, or logging command output. Don't cache failures or retry the request automatically.

The agent gets the same caching behavior as other CLI clients and wsproxy. `CODER_AGENT_HEADER_COMMAND` selects the command to run; JWT expiration controls when its output is refreshed. Each command provider has its own cache. Clients that don't use this transport are unchanged.

### Cache and concurrent requests

The command provider owns a `singleflight.Group`. Every `Headers` call checks and refreshes the cache inside `Do` with the same key. Nothing outside that function reads or writes the cache, so there is no separate cache mutex.

Published maps are immutable; each caller receives a clone after `Do` returns. Cache hits do not extend expiration, and there is no background timer. A token already within the 10-second margin causes the next request to rerun the command.

### Testing

Use a fake clock and a local helper command to test JWT cache/refresh, static output, and command failure. Keep the existing CLI, agent, wsproxy, and DERP-header tests passing.

## 🏛️ Key decisions

We will read unverified JWT claims using go-jose, the same library as `coderd`. The earliest `exp` controls refresh of the whole header set, with a fixed 10-second margin. This is only a cache hint, not signature verification or authentication. We will not change the output format.

### Other projects

[Kubernetes exec plugins](https://github.com/kubernetes/client-go/blob/v0.32.0/plugin/pkg/client/auth/exec/exec.go#L338-L409) run an external command to obtain a token or client certificate. They can [return an expiration timestamp](https://github.com/kubernetes/client-go/blob/v0.32.0/pkg/apis/clientauthentication/v1/types.go#L54-L68), which tells the client when to run the command again. We will infer expiration from JWT values rather than require a new output format.

Docker separates [custom HTTP headers](https://docs.docker.com/reference/cli/docker/#custom-http-headers) from [credential helpers](https://docs.docker.com/reference/cli/docker/login/#credential-helper-protocol). Custom headers are static settings for requests to the daemon. Credential helpers return registry login credentials, not arbitrary headers. Neither is a direct replacement for our header command.

[Bazel credential helpers](https://github.com/EngFlow/credential-helper-spec/blob/7df9bef60ef05636fd93114a17a7b2ea08143af6/schemas/get-credentials-response.schema.json#L6-L22) are closer to what we have: they return arbitrary headers and can include an expiration. We will read JWT expiration instead of adding separate expiration metadata.

## ⚠️ Risks

Opaque credentials and JWTs without a usable expiration remain static. Helpers returning tokens within the refresh margin will run on every request; concurrent requests can share an execution.

## ✅ Sign-off

- [ ] [Eng owner] considers the design complete enough to implement
- [ ] [Reviewer] has reviewed How it works and has no blocking comments
