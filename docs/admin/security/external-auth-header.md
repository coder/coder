# External authentication header

> [!CAUTION]
> Misconfiguring this feature is a full-impersonation footgun. Read the
> threat model before enabling it. See
> [issue #8889](https://github.com/coder/coder/issues/8889) for the
> original design discussion.

Coder can trust an upstream authentication gateway that sits in front
of `coder server` to assert the authenticated user via a request
header. This is intended for deployments where every request flows
through an identity-aware reverse proxy (for example, Envoy with a
custom external authorization plugin) and Coder runs on a private
network.

## How it works

When the feature is enabled, Coder accepts the `Coder-Authorization`
header on requests that originate from a configured CIDR allowlist.
The header asserts the user identity that Coder should run the
request as:

```http
Coder-Authorization: Basic Username=alice
Coder-Authorization: Basic UserEmail=alice@example.com
```

Either `Username` or `UserEmail` must be present. Other fields are
accepted and ignored for forward compatibility. The header value is
case-insensitive on field names and tolerant of whitespace.

When Coder accepts the assertion, it:

1. Looks up the user via `GetUserByEmailOrUsername` (deleted users
   are not found).
2. Builds an in-memory API key bound to that user. The key is never
   persisted, no session token is issued, and the user's existing
   sessions are not touched.
3. Loads the user's roles, groups, and status, and runs the request
   under that authorization context.

If the header is absent, malformed, or the request did not come from
a trusted origin, Coder falls back to the normal session-token flow.

## Threat model

Coder fully trusts the header on trusted origins. Anyone who can
deliver a request with a `Coder-Authorization` header from one of
those origins can impersonate any user.

Run Coder behind a hardened reverse proxy and pick exactly one of:

- Bind `coder server` to `localhost` and run the proxy on the same
  host. Trust `127.0.0.1/32` and `::1/128`.
- Use mTLS between the proxy and Coder over a private network and
  trust the proxy's network range.

Never list a network that contains untrusted clients. The proxy is
responsible for stripping any inbound `Coder-Authorization` header
before it is allowed to set its own. Coder does not strip it for you.

User auto-creation is intentionally not supported. Provision users
ahead of time using SCIM, the API, or `coder users create`.

## Configuration

Two flags on `coder server` enable the feature. Both are required:

```sh
coder server \
  --dangerous-allow-external-auth-header \
  --dangerous-external-auth-header-trusted-origins=127.0.0.0/8,::1/128
```

Or via environment:

```sh
CODER_DANGEROUS_ALLOW_EXTERNAL_AUTH_HEADER=true
CODER_DANGEROUS_EXTERNAL_AUTH_HEADER_TRUSTED_ORIGINS=127.0.0.0/8,::1/128
```

Setting `--dangerous-allow-external-auth-header` without trusted
origins is a no-op: Coder logs a warning at startup and the header
will never be honored.

## Example: Envoy with external authorization

The proxy must be configured to set the header itself, not forward
one supplied by the client. A minimal Envoy `ext_authz` filter that
populates the header from an upstream identity decision looks like:

```yaml
http_filters:
  - name: envoy.filters.http.ext_authz
    typed_config:
      "@type": type.googleapis.com/envoy.extensions.filters.http.ext_authz.v3.ExtAuthz
      transport_api_version: V3
      grpc_service:
        envoy_grpc:
          cluster_name: ext-authz
      authorization_response:
        allowed_upstream_headers:
          patterns:
            - exact: coder-authorization
```

The `ext-authz` service authenticates the user (cookie, SAML, OIDC,
mTLS, etc.) and responds with a `Coder-Authorization: Basic
Username=...` header that Envoy then injects into the upstream
request. Coder, bound to `localhost` and trusting `127.0.0.0/8`,
accepts the assertion.

## Limitations

- Only the `Basic` scheme is supported today. Future schemes may add
  signed assertions (HMAC, JWT) for deployments that cannot rely on
  network-level trust alone.
- The `ActiveSession` and `TokenName` fields from the original
  proposal are accepted and ignored.
- The header bypasses Coder's own session refresh and OAuth refresh
  paths. The proxy is responsible for keeping the upstream session
  alive.
