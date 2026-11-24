---
title: Security configuration reference
---

This reference is for platform administrators who configure the Coder control plane.
It lists the security-relevant server settings and the values that weaken or strengthen them.

Use it to review a deployment's configuration.
For the threat model behind each area and broader guidance, such as identity provider setup and template review, refer to [Security best practices](security-best-practices.md).

> [!NOTE]
> This page is configuration guidance.
> It does not map settings to any compliance framework, and applying it does not certify a deployment.

Each setting is a `coder server` flag with a matching environment variable.
For the full description of each setting, follow its link to the [server CLI reference](../../reference/cli/server/index.md).
For each setting's default value and YAML key, refer to the [Configuration reference](../setup/configuration-reference.md).

## TLS

Coder's TLS settings apply only when the control plane serves TLS itself, which you turn on with [`--tls-enable`](../../reference/cli/server/index.md#--tls-enable).
If a reverse proxy or load balancer terminates TLS in front of Coder, set the client-facing TLS version and ciphers on the proxy.
To also encrypt traffic from the proxy to Coder, as [Encryption in transit](security-best-practices.md#encryption-in-transit) recommends, turn on `--tls-enable` and use the following settings.

| Setting                                                                                            | Environment variable               | Effect of changing it                                                                                                                           |
|----------------------------------------------------------------------------------------------------|------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------|
| [`--tls-min-version`](../../reference/cli/server/index.md#--tls-min-version)                       | `CODER_TLS_MIN_VERSION`            | `tls13` strengthens the default by accepting only TLS 1.3 clients. `tls10` and `tls11` weaken it by also accepting TLS 1.0 and TLS 1.1 clients. |
| [`--tls-ciphers`](../../reference/cli/server/index.md#--tls-ciphers)                               | `CODER_TLS_CIPHERS`                | An empty value uses only the cipher suites Go marks as secure. A list restricts the server to those suites.                                     |
| [`--tls-allow-insecure-ciphers`](../../reference/cli/server/index.md#--tls-allow-insecure-ciphers) | `CODER_TLS_ALLOW_INSECURE_CIPHERS` | `true` lets `--tls-ciphers` include cipher suites that Go marks as insecure.                                                                    |
| [`--tls-client-auth`](../../reference/cli/server/index.md#--tls-client-auth)                       | `CODER_TLS_CLIENT_AUTH`            | `require-and-verify` requires every client to present a certificate signed by `--tls-client-ca-file`.                                           |

The server refuses to start in these cases:

- `--tls-ciphers` lists an insecure cipher suite and `--tls-allow-insecure-ciphers` is `false`.
- `--tls-ciphers` is set and `--tls-min-version` is `tls13`, because TLS 1.3 cipher suites aren't configurable.

> [!WARNING]
> Setting `--tls-min-version` to `tls10` or `tls11`, or setting `--tls-allow-insecure-ciphers` to `true`, lets clients connect with protocol versions or cipher suites that have known weaknesses.
> An attacker who can observe or intercept traffic has a better chance of decrypting or tampering with it.
> Coder logs a warning for each insecure cipher suite at startup, even when you allow them.

To have browsers always use HTTPS, set [`--strict-transport-security`](../../reference/cli/server/index.md#--strict-transport-security) to a non-zero max age in seconds.
Its default, `0`, sends no `Strict-Transport-Security` header.

## Sessions and tokens

| Setting                                                                                                    | Environment variable                   | Effect of changing it                                                                                               |
|------------------------------------------------------------------------------------------------------------|----------------------------------------|---------------------------------------------------------------------------------------------------------------------|
| [`--session-duration`](../../reference/cli/server/index.md#--session-duration)                             | `CODER_SESSION_DURATION`               | A shorter duration signs out idle browser sessions sooner.                                                          |
| [`--disable-session-expiry-refresh`](../../reference/cli/server/index.md#--disable-session-expiry-refresh) | `CODER_DISABLE_SESSION_EXPIRY_REFRESH` | `true` ends every browser session when `--session-duration` elapses, even while the user is active.                 |
| [`--default-token-lifetime`](../../reference/cli/server/index.md#--default-token-lifetime)                 | `CODER_DEFAULT_TOKEN_LIFETIME`         | Sets the lifetime of tokens created without an explicit duration, such as when you run `coder login`.               |
| [`--max-token-lifetime`](../../reference/cli/server/index.md#--max-token-lifetime)                         | `CODER_MAX_TOKEN_LIFETIME`             | A shorter value caps how long users can make their API tokens last.                                                 |
| [`--max-admin-token-lifetime`](../../reference/cli/server/index.md#--max-admin-token-lifetime)             | `CODER_MAX_ADMIN_TOKEN_LIFETIME`       | Caps how long administrators can make their API tokens last.                                                        |
| [`--disable-password-auth`](../../reference/cli/server/index.md#--disable-password-auth)                   | `CODER_DISABLE_PASSWORD_AUTH`          | `true` turns off password sign-in for all users, including owners, so users sign in through your identity provider. |

While a browser session is in use, Coder extends its expiry by `--session-duration`.
API tokens keep their fixed lifetime and aren't extended by use.

The default `--max-token-lifetime` lets users create API tokens that effectively never expire.
Lower it to match your token rotation policy.
To limit what a token can do and which resources it can reach, create it with [scopes and allow lists](../users/sessions-tokens.md#api-key-scopes).

> [!IMPORTANT]
> Before you set `--disable-password-auth`, confirm that at least one owner can sign in through your identity provider.

## Sign-in consent and classification banners

To point users to a consent or acceptable-use notice before they sign in, set [`--terms-of-service-url`](../../reference/cli/server/index.md#--terms-of-service-url) (`CODER_TERMS_OF_SERVICE_URL`) to the notice's URL.
The web sign-in page then shows a link to the URL and an **I agree** button that users select before they can sign in.
The setting is empty by default, so the sign-in page shows no notice.

The notice is shown by the web sign-in page only.
Coder doesn't enforce it on the server, so API, CLI, and token-based access don't show it.

To show a persistent notice, such as the classification of the network the deployment runs on, add an [announcement banner](../setup/appearance.md#announcement-banners).
Announcement banners are a Premium feature that owners configure in the dashboard.
They appear at the top of dashboard pages after a user signs in, not on the sign-in page.

## Audit logging

Audit logs are a Premium feature.
Coder records them automatically when a Premium license is active, and there's no setting to turn them on.
Refer to [Audit logs](audit-logs.md) for the events Coder records.

To control how long Coder keeps audit log entries in its database, set [`--audit-logs-retention`](../../reference/cli/server/index.md#--audit-logs-retention) (`CODER_AUDIT_LOGS_RETENTION`) to a duration.
The default, `0`, keeps entries indefinitely.
Any other duration deletes entries older than that duration.

If you set a retention period, Coder recommends at least a year, or longer if your organization's audit requirements call for it.
Refer to [Data retention](../setup/data-retention.md) for how purging works.

Coder also writes each audit event to the server logs.
The copy you export from the server logs has its own retention period, separate from the database's.
[Control plane logs and audit logs](security-best-practices.md#control-plane-logs-and-audit-logs) recommends keeping that copy for at least two years, ideally five, in a system that resists tampering.

## Network isolation

By default, Coder tries to connect clients and workspaces directly, peer to peer, and falls back to relaying traffic through the control plane.
All workspace traffic is encrypted with WireGuard either way.
To set up direct connections, clients and workspaces query Session Traversal Utilities for Network Address Translation (STUN) servers to learn their public addresses.
By default, they query Google's public STUN servers.

| Setting                                                                                            | Environment variable               | Effect of changing it                                                                                                                                                                    |
|----------------------------------------------------------------------------------------------------|------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [`--block-direct-connections`](../../reference/cli/server/index.md#--block-direct-connections)     | `CODER_BLOCK_DIRECT`               | `true` relays every CLI workspace connection through Coder or your configured relay servers, never peer to peer.                                                                         |
| [`--derp-server-stun-addresses`](../../reference/cli/server/index.md#--derp-server-stun-addresses) | `CODER_DERP_SERVER_STUN_ADDRESSES` | `disable` stops clients and workspaces from contacting STUN servers, so most connections are relayed, which is slower than connecting directly. You can also list your own STUN servers. |
| [`--browser-only`](../../reference/cli/server/index.md#--browser-only)                             | `CODER_BROWSER_ONLY`               | `true` allows workspace connections only through the browser. This is a Premium setting.                                                                                                 |
| [`--disable-path-apps`](../../reference/cli/server/index.md#--disable-path-apps)                   | `CODER_DISABLE_PATH_APPS`          | `true` serves workspace apps only from subdomains of your wildcard access URL.                                                                                                           |

Workspaces that are already running might still contact STUN servers after you set `--block-direct-connections`, until they restart.
New connections are relayed regardless.

[STUN](security-best-practices.md#stun) recommends keeping the default STUN servers unless you consider your public IP addresses and ports sensitive.
For why path-based apps are a risk, refer to [Disable path-based apps](security-best-practices.md#disable-path-based-apps).
To limit what workspaces can reach on your network, refer to [Workspaces](security-best-practices.md#workspaces).
For a deployment without internet access, refer to [Air-gapped deployments](../../install/prepare/airgap.md).

## Telemetry and update checks

| Setting                                                                | Environment variable     | Effect of changing it                                           |
|------------------------------------------------------------------------|--------------------------|-----------------------------------------------------------------|
| [`--telemetry`](../../reference/cli/server/index.md#--telemetry)       | `CODER_TELEMETRY_ENABLE` | `false` stops the deployment from sending usage data to Coder.  |
| [`--update-check`](../../reference/cli/server/index.md#--update-check) | `CODER_UPDATE_CHECK`     | `false` stops the daily check of GitHub for new Coder releases. |

Release builds turn on both settings by default.
The [Configuration reference](../setup/configuration-reference.md#update-check) lists `false` as the default for `--update-check` because it's generated from a development build, where the check is off.

Refer to [Telemetry](../setup/telemetry.md) for what Coder collects.

## Database

Coder stores its data in PostgreSQL.
The following sections cover how to encrypt the connection to the database and the sensitive values stored in it.

### Connection encryption

Set the PostgreSQL connection with [`--postgres-url`](../../reference/cli/server/index.md#--postgres-url) (`CODER_PG_CONNECTION_URL`).
The `sslmode` parameter in the URL controls whether Coder encrypts the connection and checks the server's certificate:

| `sslmode`             | Encrypted | Certificate checked                                                     |
|-----------------------|-----------|-------------------------------------------------------------------------|
| `verify-full`         | Yes       | Signed by a trusted certificate authority (CA) and matches the hostname |
| `verify-ca`           | Yes       | Signed by a trusted CA                                                  |
| `require`, or omitted | Yes       | No                                                                      |
| `disable`             | No        | No                                                                      |

Use `sslmode=verify-full` so Coder only connects to a PostgreSQL server whose certificate is valid for the expected hostname, for example:

```txt
postgres://coder:<password>@db.example.com:5432/coder?sslmode=verify-full
```

When `--postgres-url` is empty, Coder runs a built-in PostgreSQL database on the same host and connects to it over `localhost` without TLS.

### Encryption at rest

By default, Coder stores external tokens and other sensitive values in the database in plaintext.
To encrypt them, set [`--external-token-encryption-keys`](../../reference/cli/server/index.md#--external-token-encryption-keys) (`CODER_EXTERNAL_TOKEN_ENCRYPTION_KEYS`).
This is a Premium setting.
Refer to [Database encryption](database-encryption.md#turn-on-encryption) to generate keys and turn encryption on.

## Cookies and sign-up limits

| Setting                                                                                                                                | Environment variable                                 | Effect of changing it                                                                                                                                                 |
|----------------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [`--host-prefix-cookie`](../../reference/cli/server/index.md#--host-prefix-cookie)                                                     | `CODER_HOST_PREFIX_COOKIE`                           | `true` adds the `__Host-` prefix to cookies so only the right domain can set them. Coder recommends enabling it. Workspaces built before release 2.31 need a restart. |
| [`--oidc-allow-signups`](../../reference/cli/server/index.md#--oidc-allow-signups)                                                     | `CODER_OIDC_ALLOW_SIGNUPS`                           | `false` stops new users from signing up with OIDC.                                                                                                                    |
| [`--oidc-email-domain`](../../reference/cli/server/index.md#--oidc-email-domain)                                                       | `CODER_OIDC_EMAIL_DOMAIN`                            | Limits OIDC sign-in to the listed email domains.                                                                                                                      |
| [`--oidc-allowed-groups`](../../reference/cli/server/index.md#--oidc-allowed-groups)                                                   | `CODER_OIDC_ALLOWED_GROUPS`                          | Limits OIDC sign-in to members of the listed groups.                                                                                                                  |
| [`--oauth2-github-allowed-orgs`](../../reference/cli/server/index.md#--oauth2-github-allowed-orgs)                                     | `CODER_OAUTH2_GITHUB_ALLOWED_ORGS`                   | Limits GitHub sign-in to members of the listed organizations.                                                                                                         |
| [`--oauth2-github-allowed-teams`](../../reference/cli/server/index.md#--oauth2-github-allowed-teams)                                   | `CODER_OAUTH2_GITHUB_ALLOWED_TEAMS`                  | Limits GitHub sign-in to members of the listed teams, in `<organization-name>/<team-slug>` form.                                                                      |
| [`--oauth2-github-default-provider-enable`](../../reference/cli/server/index.md#--oauth2-github-default-provider-enable)               | `CODER_OAUTH2_GITHUB_DEFAULT_PROVIDER_ENABLE`        | `false` turns off the GitHub sign-in provider that Coder manages.                                                                                                     |
| [`--external-auth-github-default-provider-enable`](../../reference/cli/server/index.md#--external-auth-github-default-provider-enable) | `CODER_EXTERNAL_AUTH_GITHUB_DEFAULT_PROVIDER_ENABLE` | `false` turns off the GitHub external authentication provider that Coder manages.                                                                                     |

## Workspace access

| Setting                                                                                                    | Environment variable                   | Effect of changing it                                                                                                                    |
|------------------------------------------------------------------------------------------------------------|----------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------|
| [`--disable-owner-workspace-access`](../../reference/cli/server/index.md#--disable-owner-workspace-access) | `CODER_DISABLE_OWNER_WORKSPACE_ACCESS` | `true` removes the owner role's permission to use SSH, apps, and the terminal in workspaces. Owners keep access to their own workspaces. |
| [`--disable-workspace-sharing`](../../reference/cli/server/index.md#--disable-workspace-sharing)           | `CODER_DISABLE_WORKSPACE_SHARING`      | `true` lets only workspace owners use SSH, apps, and the terminal in their workspaces.                                                   |

## Diagnostic endpoints

Keep the [`--pprof-address`](../../reference/cli/server/index.md#--pprof-address) (default `127.0.0.1:6060`) and [`--prometheus-address`](../../reference/cli/server/index.md#--prometheus-address) (default `127.0.0.1:2112`) bound to localhost when you enable [`--pprof-enable`](../../reference/cli/server/index.md#--pprof-enable) or [`--prometheus-enable`](../../reference/cli/server/index.md#--prometheus-enable).
In production, leave [`--swagger-enable`](../../reference/cli/server/index.md#--swagger-enable) and [`--experiments`](../../reference/cli/server/index.md#--experiments) off, because experiments aren't ready for production.

## Settings that weaken defaults

The settings in the following table are off or empty by default.

> [!CAUTION]
> Change these settings only when you need to.
> Each one removes or loosens a protection that Coder applies by default and can expose user sessions, sign-in, network traffic, or internal services to attack.

| Setting                                                                                                                            | Environment variable                               | Effect of changing it                                                                                                                                                                                                                              |
|------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [`--tls-allow-insecure-ciphers`](../../reference/cli/server/index.md#--tls-allow-insecure-ciphers)                                 | `CODER_TLS_ALLOW_INSECURE_CIPHERS`                 | Allows cipher suites that Go marks as insecure.                                                                                                                                                                                                    |
| [`--dangerous-allow-path-app-sharing`](../../reference/cli/server/index.md#--dangerous-allow-path-app-sharing)                     | `CODER_DANGEROUS_ALLOW_PATH_APP_SHARING`           | Lets users share path-based apps with other users.                                                                                                                                                                                                 |
| [`--dangerous-allow-path-app-site-owner-access`](../../reference/cli/server/index.md#--dangerous-allow-path-app-site-owner-access) | `CODER_DANGEROUS_ALLOW_PATH_APP_SITE_OWNER_ACCESS` | Lets owners open path-based apps in workspaces they don't own.                                                                                                                                                                                     |
| [`--dangerous-oidc-skip-issuer-checks`](../../reference/cli/server/index.md#--dangerous-oidc-skip-issuer-checks)                   | `CODER_DANGEROUS_OIDC_SKIP_ISSUER_CHECKS`          | Stops Coder from requiring the OpenID Connect (OIDC) issuer URL to match the ID token and provider configuration.                                                                                                                                  |
| [`--oidc-ignore-email-verified`](../../reference/cli/server/index.md#--oidc-ignore-email-verified)                                 | `CODER_OIDC_IGNORE_EMAIL_VERIFIED`                 | Coder accepts email addresses that the identity provider hasn't verified.                                                                                                                                                                          |
| [`--oauth2-github-allow-everyone`](../../reference/cli/server/index.md#--oauth2-github-allow-everyone)                             | `CODER_OAUTH2_GITHUB_ALLOW_EVERYONE`               | Any GitHub account can sign in.                                                                                                                                                                                                                    |
| [`--email-tls-skip-verify`](../../reference/cli/server/index.md#--email-tls-skip-verify)                                           | `CODER_EMAIL_TLS_SKIPVERIFY`                       | Coder doesn't check the mail server's certificate. The deprecated [`--notifications-email-tls-skip-verify`](../../reference/cli/server/index.md#--notifications-email-tls-skip-verify) (`CODER_NOTIFICATIONS_EMAIL_TLS_SKIPVERIFY`) does the same. |
| [`--additional-csp-policy`](../../reference/cli/server/index.md#--additional-csp-policy)                                           | `CODER_ADDITIONAL_CSP_POLICY`                      | Adds values to the Content Security Policy (CSP) that Coder sends to protect against cross-site scripting (XSS), which can widen what browsers let Coder pages load.                                                                               |
| [`--mcp-allowed-private-cidrs`](../../reference/cli/server/index.md#--mcp-allowed-private-cidrs)                                   | `CODER_MCP_ALLOWED_PRIVATE_CIDRS`                  | Lets Coder connect to Model Context Protocol (MCP) servers in the listed private or reserved IP ranges, which it otherwise blocks to prevent server-side request forgery (SSRF).                                                                   |
| [`--ai-gateway-proxy-allowed-private-cidrs`](../../reference/cli/server/index.md#--ai-gateway-proxy-allowed-private-cidrs)         | `CODER_AI_GATEWAY_PROXY_ALLOWED_PRIVATE_CIDRS`     | Lets AI Gateway Proxy connect to the listed private or reserved IP ranges, which it otherwise blocks to prevent SSRF. This is a Premium setting.                                                                                                   |

`--tls-min-version` values below `tls12` also weaken the default.
Refer to [TLS](#tls).
