---
title: Security - best practices
---

December 16, 2024

---

This best practices guide is separated into parts to help you secure aspects of your Coder deployment.

Each section briefly introduces each threat model, then suggests steps or concepts to help implement security improvements such as authentication and encryption.

As with any security guide, the steps and suggestions outlined in this document are not meant to be exhaustive and do not offer any guarantee.

Where a `coder server` setting applies a section's guidance, the section names it, and [Settings that weaken defaults](#settings-that-weaken-defaults) lists the settings to review when you audit a deployment.
For each setting's default value, environment variable, and YAML key, refer to the [Configuration reference](../setup/configuration-reference.md).

## Control plane

The control plane is the core of a Coder deployment.

If the control plane is compromised in a security incident, it can affect every other part of your deployment.
Even a successful read-only attack against the control plane could result in a complete compromise of the Coder deployment if credentials are stolen.

### User authentication

Configure [OpenID Connect (OIDC) authentication](../users/oidc-auth/index.md) against your organization’s Identity Provider (IdP), such as Okta, to allow single-sign on.

1. Enable and require two-factor authentication in your identity provider.
1. Enable [IdP Sync](../users/idp-sync.md) to manage users’ roles and groups in Coder.
1. Use System for Cross-domain Identity Management (SCIM) to automatically suspend users when they leave the organization.

This allows you to manage user credentials according to your company’s central requirements, such as password complexity, 2FA, PassKeys, and others.

Using IdP sync and SCIM means that the central Identity Provider is the source of truth, so that when users change roles or leave, their permissions in Coder are automatically up to date.

After you confirm that at least one owner can sign in through your identity provider, set [`--disable-password-auth`](../../reference/cli/server/index.md#--disable-password-auth) to `true` to turn off password sign-in for all users, including owners.

To stop new users from signing up through OIDC, set [`--oidc-allow-signups`](../../reference/cli/server/index.md#--oidc-allow-signups) to `false`.
To limit OIDC sign-in to listed email domains or groups, set [`--oidc-email-domain`](../../reference/cli/server/index.md#--oidc-email-domain) or [`--oidc-allowed-groups`](../../reference/cli/server/index.md#--oidc-allowed-groups).
If you use GitHub sign-in, limit it to members of listed organizations or teams with [`--oauth2-github-allowed-orgs`](../../reference/cli/server/index.md#--oauth2-github-allowed-orgs) or [`--oauth2-github-allowed-teams`](../../reference/cli/server/index.md#--oauth2-github-allowed-teams).
If you don't use the GitHub sign-in and external authentication providers that Coder manages, set [`--oauth2-github-default-provider-enable`](../../reference/cli/server/index.md#--oauth2-github-default-provider-enable) and [`--external-auth-github-default-provider-enable`](../../reference/cli/server/index.md#--external-auth-github-default-provider-enable) to `false`.

#### Sessions and API tokens

Coder extends a browser session's expiry by [`--session-duration`](../../reference/cli/server/index.md#--session-duration) while the session is in use.
To end every browser session when that duration elapses, even while the user is active, set [`--disable-session-expiry-refresh`](../../reference/cli/server/index.md#--disable-session-expiry-refresh) to `true`.
To make sure only the Coder domain can set Coder's cookies, set [`--host-prefix-cookie`](../../reference/cli/server/index.md#--host-prefix-cookie) to `true`, then restart workspaces built before release 2.31.

API tokens keep a fixed lifetime and aren't extended by use.
The default [`--max-token-lifetime`](../../reference/cli/server/index.md#--max-token-lifetime) lets users create tokens that effectively never expire, so lower it to match your token rotation policy.
[`--max-admin-token-lifetime`](../../reference/cli/server/index.md#--max-admin-token-lifetime) caps the lifetime of administrators' tokens, and [`--default-token-lifetime`](../../reference/cli/server/index.md#--default-token-lifetime) sets the lifetime of tokens created without an explicit duration, such as when you run `coder login`.
To limit what a token can do and which resources it can reach, create it with [scopes and allow lists](../users/sessions-tokens.md#api-key-scopes).

#### Sign-in notices

To show a consent or acceptable-use notice before users sign in, set [`--terms-of-service-url`](../../reference/cli/server/index.md#--terms-of-service-url) to the notice's URL.
The web sign-in page then shows a link to the notice and an **I agree** button that users select before they can sign in.
Coder doesn't enforce the notice on the server, so API, CLI, and token-based access don't show it.

To show a persistent notice after users sign in, such as the classification of the network the deployment runs on, add an [announcement banner](../setup/appearance.md#announcement-banners).
Announcement banners are a Premium feature, and they appear at the top of dashboard pages, not on the sign-in page.

### Encryption in transit

Place Coder behind a TLS-capable reverse-proxy/load balancer and enable [Strict Transport Security](../../reference/cli/server/index.md#--strict-transport-security) so that connections from end users are always encrypted.

Enable [TLS](../../reference/cli/server/index.md#--tls-address) on the control plane and encrypt traffic from the reverse-proxy/load balancer to the control plane, so that even if an attacker gains access to your network, they will not be able to snoop on control plane traffic.

Set the client-facing TLS version and ciphers on the reverse proxy or load balancer.
Coder's own TLS settings apply only when [`--tls-enable`](../../reference/cli/server/index.md#--tls-enable) is `true`.
Set [`--tls-min-version`](../../reference/cli/server/index.md#--tls-min-version) to `tls12` or `tls13`, and leave [`--tls-ciphers`](../../reference/cli/server/index.md#--tls-ciphers) empty so Coder uses only the cipher suites Go marks as secure.
To require every client to present a certificate, set [`--tls-client-auth`](../../reference/cli/server/index.md#--tls-client-auth) to `require-and-verify` and [`--tls-client-ca-file`](../../reference/cli/server/index.md#--tls-client-ca-file) to the certificate authority (CA) that signs the client certificates.

`--strict-transport-security` takes a max age in seconds.
Its default, `0`, sends no `Strict-Transport-Security` header.

### Encryption at rest

The control plane persists no state locally.
No action is required.

### Control plane logs and audit logs

Capture the logging output of all control plane instances and persist them.

Retain all logs for a minimum of thirty days, ideally ninety days.
Filter audit logs (which have `msg: audit_log`) and retain them for a minimum of two years (ideally five years) in a secure system that resists tampering.

If a security incident with Coder does occur, audit logs are invaluable in determining the nature and scope of the impact.

[Audit logs](audit-logs.md) are a Premium feature.
Coder keeps audit log entries in its database indefinitely unless you set [`--audit-logs-retention`](../../reference/cli/server/index.md#--audit-logs-retention).
If you set it, keep entries for at least a year, or longer if your organization's audit requirements call for it.
This database retention period is separate from the retention period for the copy you export from the server logs.
Refer to [Data retention](../setup/data-retention.md) for how purging works.

### Disable path-based apps

For production deployments, we recommend that you disable path-based apps after you've configured a wildcard access URL.

Path-based apps share the same origin as the Coder API, which can be convenient for trialing Coder, but can expose the deployment to cross-site-scripting (XSS) attacks in production.
A malicious workspace could reuse Coder cookies to call the API or interact with other workspaces owned by the same user.

1. [Enable sub-domain apps with a wildcard DNS record](../setup/index.md#wildcard-access-url) (like `*.coder.example.com`)

1. Disable path-based apps:

   ```sh
   coder server --disable-path-apps
   # or
   export CODER_DISABLE_PATH_APPS=true
   ```

By default, Coder mitigates the impact of having path-based apps enabled, but we still recommend disabling it to prevent malicious workspaces accessing other workspaces owned by the same user or performing requests against the Coder API.

If you do keep path-based apps enabled:

- Path-based apps cannot be shared with other users unless you start `coder server` with `--dangerous-allow-path-app-sharing`.
- Users with the site `owner` role cannot use their admin privileges to access path-based apps for workspaces unless `coder server` is started with `--dangerous-allow-path-app-site-owner-access`.

### Diagnostics, telemetry, and update checks

Keep [`--pprof-address`](../../reference/cli/server/index.md#--pprof-address) and [`--prometheus-address`](../../reference/cli/server/index.md#--prometheus-address) bound to localhost when you turn on [`--pprof-enable`](../../reference/cli/server/index.md#--pprof-enable) or [`--prometheus-enable`](../../reference/cli/server/index.md#--prometheus-enable).
In production, leave [`--swagger-enable`](../../reference/cli/server/index.md#--swagger-enable) off and [`--experiments`](../../reference/cli/server/index.md#--experiments) empty, because experiments aren't ready for production.

By default, a deployment sends usage data to Coder, and release builds also check GitHub once a day for new Coder releases.
To turn these off, set [`--telemetry`](../../reference/cli/server/index.md#--telemetry) or [`--update-check`](../../reference/cli/server/index.md#--update-check) to `false`.
The [Configuration reference](../setup/configuration-reference.md#update-check) lists `false` as the default for `--update-check` because it's generated from a development build, where the check is off.
Refer to [Telemetry](../setup/telemetry.md) for what Coder collects.

## PostgreSQL

PostgreSQL is the persistent datastore underlying the entire Coder deployment.
If the database is compromised, it might leave every other part of your deployment vulnerable.

Coder session tokens and API keys are stored only as SHA-256 hashes of random secrets, so a read-only compromise of the database is unlikely to allow an attacker to log into Coder.
However, the database contains the Terraform state for all workspaces, OIDC tokens, and agent tokens, so it is possible that a read-only attack could enable lateral movement to other systems.

A successful attack that modifies database state could be escalated to a full takeover of an owner account in Coder which could lead to a complete compromise of the Coder deployment.

### Authentication

1. Generate a strong, random password for accessing PostgreSQL and store it securely.

1. Use environment variables to pass the PostgreSQL URL to Coder.

1. If on Kubernetes, use a Kubernetes secret to set the environment variable.

### Encryption in transit

Enable TLS on PostgreSQL and set `sslmode=verify-full` in your [postgres URL](../../reference/cli/server/index.md#--postgres-url) on the control plane.
This configures the control plane to only establish TLS connections to PostgreSQL and check that the PostgreSQL server’s certificate is valid and matches the expected hostname.

With `sslmode=require`, or with no `sslmode`, Coder encrypts the connection but doesn't check the server's certificate.
If `--postgres-url` is empty, Coder runs a built-in PostgreSQL database on the same host and connects to it over `localhost` without TLS.

### Encryption at rest

Run PostgreSQL on servers with full disk encryption enabled and configured.

Coder supports [encrypting some particularly sensitive data](database-encryption.md) including OIDC tokens using an encryption key managed independently of the database, so even a user with full administrative privileges on the PostgreSQL server(s) cannot read the data without the separate key.
This is a Premium feature that you turn on with [`--external-token-encryption-keys`](../../reference/cli/server/index.md#--external-token-encryption-keys).
Without it, Coder stores these values in the database in plaintext.

If you use this feature:

1. Generate a random encryption key and store it in a central secrets management system like Vault.

1. Inject the secret using an environment variable.

   - If you're using Kubernetes, use a Kubernetes secret rather than including the secret directly in the podspec.

1. Follow your organization's policies about key rotation on a fixed schedule.

   - If you suspect the key has been leaked or compromised, [rotate the key immediately](database-encryption.md#rotate-keys).

## Provisioner daemons

Provisioner daemons are deployed with credentials that give them power to make requests to cluster/cloud APIs.

If one of those credentials is compromised, the potential severity of the compromise depends on the permissions granted to the credentials, but will almost certainly include code execution inside the cluster/cloud since the whole purpose of Coder is to deploy workspaces in the cluster/cloud that can run developer code.

In addition, provisioner daemons are given access to parameters entered by end users, which could include sensitive data like credentials for additional systems.

### External provisioner daemons

When Coder workspaces are deployed into multiple clusters/clouds, or workspaces are in a different cluster/cloud than the control plane, use external provisioner daemons.

Running provisioner daemons within the same cluster/cloud as the workspaces they provision:

- Allows you to use infrastructure-provided credentials (see **Authentication** below) which are typically easier to manage and have shorter lifetimes than credentials issued outside the cloud/cluster.
- Means that you don’t have to open any ingress ports on the clusters/clouds that host workspaces.
  - The external provisioner daemons dial out to the control plane.
  - Provisioner daemons run in the cluster, so you don’t need to expose cluster/cloud APIs externally.
- Each cloud/cluster is isolated, so a compromise of a provisioner daemon is limited to a single cluster.

### Authentication

1. Use a [scoped key](../../install/operate/provisioners/index.md#scoped-key-recommended) to authenticate the provisioner daemons with Coder.

   These keys can only be used to authenticate provisioner daemons (not other APIs on the control plane).

1. Store the keys securely and use environment variables to pass them to the provisioner daemon.

1. If on Kubernetes, use a Kubernetes secret to set the environment variable.

1. Tag provisioners with identifiers for the specific cluster/cloud.

   This allows your templates to target a specific cluster/cloud such as for geographic proximity to the end user, or for specific features like graphics processing units (GPUs) or managed services.

1. Scope your keys to organizations and the specific cluster/cloud using the same tags when creating the keys.

   This makes sure that a compromised key will not allow an attacker to gain access to jobs for other clusters or organizations.

Provisioner daemons should have access only to cluster/cloud API credentials for the specific cluster/cloud they are for.
This guarantees that compromise of one Provisioner Daemon does not compromise all clusters/clouds.

Deploy the provisioner daemon to the cloud and leverage infrastructure-provided credentials, if available:

- [Service account tokens on Kubernetes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/)
- [AWS Identity and Access Management (IAM) roles for Amazon Elastic Compute Cloud (EC2) on AWS](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/iam-roles-for-amazon-ec2.html)
- [Attached service accounts on Google Cloud](https://cloud.google.com/iam/docs/attach-service-accounts)

### Encryption in transit

Enable TLS on the control plane and make sure you use an `https://` URL to access the control plane.

See the **Encryption in transit** subheading of the [Templates](#workspace-templates) section for more about encrypting cluster/cloud API calls.

### Encryption at rest

Run provisioner daemons only on systems with full disk encryption enabled.

- Provisioner daemons temporarily persist terraform template files and resource state to disk.
  Either of these could contain sensitive information, including credentials.

  This temporary state is on disk only while actively building workspaces, but an attacker who compromises physical disks could successfully read this information if not encrypted.

- Provisioner daemons store cached copies of Terraform provider binaries.
  These are generally not sensitive in terms of confidentiality, but it is important to maintain their integrity.
  An attacker that can modify these binaries could inject malicious code.

## Workspace proxies

Workspace proxies authenticate end users and then proxy network traffic to workspaces.

Coder takes care to make sure the user credentials processed by workspace proxies are scoped to application access and do not grant full access to the Coder API on behalf of the user.
Still, a fully compromised workspace proxy would be in a privileged position to phish unrestricted user credentials.

Workspace proxies have unrestricted access to establish encrypted tunnels to workspaces and can access any port on any running workspace.

### Authentication

1. Securely store the workspace proxy token generated by [`coder wsproxy create`](../networking/workspace-proxies.md#step-1-create-the-proxy).

1. Inject the token to the workspace proxy process via an environment variable, rather than via an argument.

1. If on Kubernetes, use a Kubernetes secret to set the environment variable.

### Encryption in transit

Enable TLS on the control plane and make sure you use an `https://` URL to access the control plane.

Communication to the proxied workspace applications is always encrypted with Wireguard.
No action is required.

### Encryption at rest

Workspace proxies persist no state locally.
No action is required.

## Workspace templates

Coder templates are executed on provisioner daemons and can include arbitrary code via the [local-exec provisioner](https://developer.hashicorp.com/terraform/language/resources/provisioners/local-exec).

Furthermore, Coder templates are designed to provision compute resources in one or more clusters/clouds, and template authors are generally in full control over code and scripts executed by the workspace agent in those compute resources.

This means that template admins have remote code execution privileges for any provisioner daemons in their organization and within any cluster/cloud those provisioner daemons are credentialed to access.

Template admin is a powerful, highly-trusted role that you should not assign lightly.
Instead of directly assigning the role to anyone who might need to edit a template, use [GitOps](#gitops) to allow users to author and edit templates.

## Secrets

Never include credentials or any other secrets directly in templates, including in `.tfvars` or other files uploaded with the template.

Instead do one of the following:

- Store secrets in a central secrets manager.

  - Access the secrets at build time via a Terraform provider.

    This can be through [Vault](https://registry.terraform.io/providers/hashicorp/vault/latest/docs) or [AWS Secrets Manager](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/secretsmanager_secret).

- Place secrets in `TF_VAR_*` environment variables.

  - Provide the secrets to the relevant Provisioner Daemons and access them via Terraform variables with `sensitive = true`.

- Use Coder parameters to accept secrets from end users at build time.

Coder does not attempt to obscure the contents of template files from users authorized to view and edit templates, so secrets included directly could inadvertently appear on screen while template authors do their work.

Template versions are persisted indefinitely in the PostgreSQL database, so if secrets are inadvertently included, they should be revoked as soon as practical.
Pushing a new template version does not expunge them from the database.
Contact support if you need assistance expunging any particularly sensitive data.

### Encryption in transit

Always use encrypted transport to access any infrastructure APIs.
Crucially, this protects confidentiality of the credentials used to access the APIs.

Configuration of this depends on the specific Terraform providers in use and is beyond the scope of this document.

### Encryption at rest

While your most privileged secrets should never be included in template files, they might inevitably contain confidential or sensitive data about your operations and/or infrastructure.

- Make sure that operators who write, review or modify Coder templates are working on laptops/workstations with full disk encryption, or do their work inside a Coder workspace with full disk encryption.
- Make sure [PostgreSQL](#postgresql) is encrypted at rest.
- Make sure any [source code repositories that store templates](#gitops) are encrypted at rest and have appropriate access controls.

### GitOps

GitOps is the practice of using a Git repository as the source of truth for operational config and reconciling the config in Git with operational systems each time the `main` (or, archaically, `master`) branch of the repository is updated.

1. Store Coder templates in a single Git repository, or a single repository per Coder organization.

   This gives you an easily browsable, auditable history of template changes and who made them.
   Coder audit logs establish who and when changes happen, but git repositories are particularly handy for analyzing exactly what changes to templates are made.

1. Use the [Coderd Terraform provider](https://registry.terraform.io/providers/coder/coderd/latest/docs/resources/template) to push changes to Coder from a continuous integration and continuous delivery (CI/CD) tool.

1. Use a Coder user account exclusively for pushing template changes, and give no human users the credentials.

   This guarantees any actions taken by the account correspond exactly to CI/CD actions from the repository and allows you to avoid granting the template admin role widely in your organization.

1. Use [GitHub branch protection](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches), or the equivalent for your source repository to enforce code review of changes to templates.

   Code review increases the chance that someone will catch a potential security bug in your template.

These protections also mitigate the risk of a single trusted insider “going rogue” and acting unilaterally to maliciously modify Coder templates.

## Workspaces

The central purpose of Coder is to give end users access to managed compute in clusters/clouds designated by Coder’s operators (like platform or developer experience teams).
End users are granted shell access and from there can execute arbitrary commands.

This means that end users have remote code execution privileges within the clusters/clouds that host Coder workspaces.

It is important to limit Coder users to trusted insiders and/or take steps to constrain malicious activity that could be undertaken from a Coder workspace.

Example constraints include:

- Network policy or segmentation
- Runtime protections on the workspace host (for example, SELinux)
- Limiting privileges of the account or role assigned to the workspace such as a service account on Kubernetes, or IAM role on public clouds
- Monitoring and/or auditing for suspicious activity such as cryptomining or exfiltration

### Owner access and workspace sharing

To stop users from sharing their workspaces with other users, set [`--disable-workspace-sharing`](../../reference/cli/server/index.md#--disable-workspace-sharing) to `true`.
Users with the `owner` role can use SSH, apps, and the terminal in every workspace, whether or not sharing is turned off.
To limit them to their own workspaces, set [`--disable-owner-workspace-access`](../../reference/cli/server/index.md#--disable-owner-workspace-access) to `true`.

### Outbound network access

Identify network assets like production systems or highly confidential datastores and configure the network to limit access from Coder workspaces.

If production systems or confidential data reside in the same cluster/cloud, use separate node pools and network boundaries.

If extraordinary access is required, follow [Zero Trust](https://en.wikipedia.org/wiki/Zero_trust_security_model) principles:

- Authenticate the user and the workspace using strong cryptography
- Apply strict authorization controls
- Audit access in a tamper resistant secure store

Consider the network assets end users will need to do their job and the level of trust the company has with them.
In-house full-time employees have different access than temporary contractors or third-party service providers.
Restrict access as appropriate.

A non-exclusive list of network assets to consider:

- Access to the public Internet
  - If end users will access the workspace over the public Internet, you must allow outbound access to establish the encrypted tunnels.
- Access to internal corporate networks
  - If end users will access the workspace over the corporate network, you must allow outbound access to establish the encrypted tunnels.
- Access to staging or production systems
- Access to confidential data (for example, payment processing data, health records, personally identifiable information)
- Access to other clusters/clouds

### Inbound network access

Coder manages inbound network access to your workspaces via a set of Wireguard encrypted tunnels.
These tunnels are established by sending outbound packets, so on stateful firewalls, disable inbound connections to workspaces to make sure inbound connections are handled exclusively by the encrypted tunnels.

To relay every CLI workspace connection through Coder or your configured relay servers, never peer to peer, set [`--block-direct-connections`](../../reference/cli/server/index.md#--block-direct-connections) to `true`.
Workspaces that are already running might still contact STUN servers until they restart, but new connections are relayed regardless.
To allow workspace connections only through the browser, set [`--browser-only`](../../reference/cli/server/index.md#--browser-only) to `true`.
This is a Premium setting.

#### DERP

[Designated Encrypted Relay for Packets (DERP)](https://tailscale.com/kb/1232/derp-servers) is a relay protocol developed by Tailscale.

The control plane and Workspace Proxies include a DERP service by default.
Tailcale also runs a set of public DERP servers, globally distributed.

All DERP messages are end-to-end encrypted, so the DERP service only learns the (public) IP addresses of the participants.

If you consider these addresses or the fact that pairs of them communicate over DERP to be sensitive, stick to the Coder-provided DERP services which run on your own infrastructure.
If not, feel free to configure Tailscale DERP servers for global coverage.

#### STUN

[Session Traversal Utilities for Network Address Translation (STUN)](https://en.wikipedia.org/wiki/STUN) is an Internet Engineering Task Force (IETF) standard protocol that allows network endpoints behind Network Address Translation (NAT) to learn their public address and port mappings.
It is an essential component of Coder’s networking to enable encrypted tunnels to be established without a relay for best performance.

Coder does not ship with a STUN service because it needs to be run directly connected to the network, not behind a reverse proxy or load balancer as Coder usually is.

STUN messages are not encrypted, but do not transmit any tunneled data, they simply query the public address and ports.
As such, a STUN service learns the public address and port information such as the address and port on the NAT device of Coder workspaces and the end user's device if STUN is configured.

Unlike DERP, it doesn’t definitively learn about communicating pairs of IPs.

If you consider the public IP and port information to be sensitive, do not use public STUN servers.

You can choose not to use any STUN servers by setting [`--derp-server-stun-addresses`](../../reference/cli/server/index.md#--derp-server-stun-addresses) to `disable`, in which case most workspace traffic will need to be relayed via DERP.
You can choose to deploy your own STUN servers, either on the public Internet, or on your corporate network and [configure Coder to use it](../../reference/cli/server/index.md#--derp-server-stun-addresses).

If you do not consider the addresses and ports to be sensitive, we recommend using the default set of STUN servers operated by Google.

#### Workspace apps

Coder workspace apps are a way to allow users to access web applications running in the workspace via the control plane or Workspace Proxy.

1. [Disable workspace apps on sub-paths](../../reference/cli/server/index.md#--disable-path-apps) of the main Coder domain name.

1. [Use a separate, wildcard domain name](../setup/index.md#wildcard-access-url) for forwarding.

   Because of the default [same-origin policy](https://en.wikipedia.org/wiki/Same-origin_policy) in browsers, serving web apps on the main Coder domain would allow those apps to send API requests to the control plane, authenticated as the logged-in user without their explicit consent.

#### Port sharing

Coder supports the option to allow users to designate specific network ports on their workspace as shared, which allows others to access those ports via the control plane.

Consider restricting the maximum sharing level for workspaces, located in the template settings for the corresponding template.

### Encryption at rest

Deploy Coder workspaces using full disk encryption for all volumes.

This mitigates attempts to recover sensitive data in the workspace by attackers who gain physical access to the disk(s).

## Settings that weaken defaults

Each setting in the following table loosens a protection that Coder applies by default.

> [!CAUTION]
> Change these settings only when you need to.
> Each one can expose user sessions, sign-in, network traffic, or internal services to attack.

| Setting                                                                                                                            | Environment variable                               | Effect of changing it                                                                                                                                                                                                                              |
|------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [`--tls-min-version`](../../reference/cli/server/index.md#--tls-min-version)                                                       | `CODER_TLS_MIN_VERSION`                            | `tls10` or `tls11` accepts clients that use TLS versions with known weaknesses.                                                                                                                                                                    |
| [`--tls-allow-insecure-ciphers`](../../reference/cli/server/index.md#--tls-allow-insecure-ciphers)                                 | `CODER_TLS_ALLOW_INSECURE_CIPHERS`                 | Lets `--tls-ciphers` include cipher suites that Go marks as insecure. Coder logs a warning for each one at startup.                                                                                                                                |
| [`--dangerous-allow-path-app-sharing`](../../reference/cli/server/index.md#--dangerous-allow-path-app-sharing)                     | `CODER_DANGEROUS_ALLOW_PATH_APP_SHARING`           | Lets users share path-based apps with other users.                                                                                                                                                                                                 |
| [`--dangerous-allow-path-app-site-owner-access`](../../reference/cli/server/index.md#--dangerous-allow-path-app-site-owner-access) | `CODER_DANGEROUS_ALLOW_PATH_APP_SITE_OWNER_ACCESS` | Lets owners open path-based apps in workspaces they don't own.                                                                                                                                                                                     |
| [`--dangerous-oidc-skip-issuer-checks`](../../reference/cli/server/index.md#--dangerous-oidc-skip-issuer-checks)                   | `CODER_DANGEROUS_OIDC_SKIP_ISSUER_CHECKS`          | Stops Coder from requiring the OIDC issuer URL to match the ID token and provider configuration.                                                                                                                                                   |
| [`--oidc-ignore-email-verified`](../../reference/cli/server/index.md#--oidc-ignore-email-verified)                                 | `CODER_OIDC_IGNORE_EMAIL_VERIFIED`                 | Coder accepts email addresses that the identity provider hasn't verified.                                                                                                                                                                          |
| [`--oauth2-github-allow-everyone`](../../reference/cli/server/index.md#--oauth2-github-allow-everyone)                             | `CODER_OAUTH2_GITHUB_ALLOW_EVERYONE`               | Any GitHub account can sign in.                                                                                                                                                                                                                    |
| [`--email-tls-skip-verify`](../../reference/cli/server/index.md#--email-tls-skip-verify)                                           | `CODER_EMAIL_TLS_SKIPVERIFY`                       | Coder doesn't check the mail server's certificate. The deprecated [`--notifications-email-tls-skip-verify`](../../reference/cli/server/index.md#--notifications-email-tls-skip-verify) (`CODER_NOTIFICATIONS_EMAIL_TLS_SKIPVERIFY`) does the same. |
| [`--additional-csp-policy`](../../reference/cli/server/index.md#--additional-csp-policy)                                           | `CODER_ADDITIONAL_CSP_POLICY`                      | Adds values to the Content Security Policy (CSP) that Coder sends to protect against XSS, which can widen what browsers let Coder pages load.                                                                                                      |
| [`--mcp-allowed-private-cidrs`](../../reference/cli/server/index.md#--mcp-allowed-private-cidrs)                                   | `CODER_MCP_ALLOWED_PRIVATE_CIDRS`                  | Lets Coder connect to Model Context Protocol (MCP) servers in the listed private or reserved IP ranges, which it otherwise blocks to prevent server-side request forgery (SSRF).                                                                   |
| [`--ai-gateway-proxy-allowed-private-cidrs`](../../reference/cli/server/index.md#--ai-gateway-proxy-allowed-private-cidrs)         | `CODER_AI_GATEWAY_PROXY_ALLOWED_PRIVATE_CIDRS`     | Lets AI Gateway Proxy connect to the listed private or reserved IP ranges, which it otherwise blocks to prevent SSRF. This is a Premium setting.                                                                                                   |
