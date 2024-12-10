# Best Practices for Coder Security

December 30, 2024

---

## Coder Server

Coder Server is the main control core of a Coder deployment. A compromise of Coder Server affects every other part of your deployment. Even a successful read-only attack against the Coder Server could result in a complete compromise of the Coder deployment if powerful credentials were stolen.

### User Authentication

Configure [OIDC authentication](../../admin/users/oidc-auth.md) against your organization’s Identity Provider (IdP), such as Okta, to allow single-sign on:

1. Enable and require two-factor authentication in your identity provider.
1. Enable [IdP Sync](../../admin/users/idp-sync.md) to manage users’ roles and groups in Coder.
1. Use SCIM to automatically suspend users when they leave the organization.
	
This allows you to manage user credentials according to your company’s central requirements (e.g. password complexity, 2FA, use of PassKeys, etc).

Using IdP sync and SCIM means that the central Identity Provider is the source of truth, so that when users change roles or leave, their permissions in Coder are automatically up to date.

### Encryption in Transit

Place Coder behind a TLS-capable reverse-proxy/load balancer, and enable [Strict Transport Security](../../reference/cli/server.md#--strict-transport-security) so that connections from end users are always encrypted.

Enable [TLS](../../reference/cli/server.md#--tls-address) on Coder Server, and encrypt traffic from the reverse-proxy/load balancer to Coder Server, so that even if an attacker gains access to your network, they will be unable to snoop on Coder Server traffic.

### Encryption at Rest

Coder Server persists no state locally. No action is required.

### Server Logs & Audit Logs

Capture the logging output of all Coder Server instances and persist them. Retain all logs for a minimum of thirty days, ideally ninety days. Filter audit logs (which have `msg: audit_log`) and retain them for a minimum of two years (ideally five years) in a secure system that resists tampering. If a security incident with Coder does occur, audit logs are invaluable in determining the nature and scope of the impact.

## PostgreSQL

PostgreSQL is the persistent data store underlying the entire Coder deployment. A compromise of the database could affect every other part of your deployment. 

Coder session tokens and API keys are salted and hashed, so read-only compromise of the database in and of itself is unlikely to allow an attacker to log into Coder. However, the database contains Terraform state for all workspaces, OIDC tokens, and agent tokens, so it remains a possibility that even a read-only attack could enable lateral movement to other systems.

A successful attack that modifies database state could be escalated to a full takeover of an owner account in Coder, and thus complete compromise of the Coder deployment.

### Authentication

Generate a strong, random password for accessing PostgreSQL and store it securely. Use environment variables to pass the PostgreSQL URL to Coder, and if on Kubernetes, use a Kubernetes Secret to set the environment variable.

### Encryption in Transit

Enable TLS on PostgreSQL, and set `sslmode=verify-full` in your [postgres URL](../../reference/cli/server.md#--postgres-url) on Coder Server. This configures Coder Server to only establish TLS connections to PostgreSQL and check that the PostgreSQL server’s certificate is valid and matches the expected hostname.

### Encryption at Rest

Run PostgreSQL on server(s) with full disk encryption enabled and configured.

Additionally, Coder supports [encrypting some particularly sensitive data](../../admin/security/database-encryption.md) including OIDC tokens using an encryption key managed independently of the database, so even a user with full administrative privileges on the PostgreSQL server(s) cannot read the data without the separate key. If you use this feature:

1. Generate the encryption key randomly and store it in a central secrets management system like Vault.
1. Inject the secret using an environment variable.
   - If in Kubernetes, use a Kubernetes Secret, rather than including the secret directly in the podspec
1. [Rotate the key immediately](../../admin/security/database-encryption.md#rotating-keys) if you suspect it has been leaked. Follow any organization policies about key rotation on a fixed schedule.

## Provisioner Daemons

Provisioner daemons are deployed with credentials that give them power to make requests to cluster/cloud APIs. The blast radius of a compromise of those credentials depends on the permissions granted the credentials, but will almost certainly include code execution inside the cluster, since the whole purpose of Coder is to deploy workspaces in the cluster/cloud that can run developer code.

In addition, provisioner daemons are given access to parameters entered by end users, which could include sensitive data like credentials for additional systems.

### External Provisioner Daemons

When Coder workspaces are deployed into multiple clusters/clouds, or workspaces are in a different cluster/cloud than the Coder Server, use external provisioner daemons. Running provisioner daemons within the same cluster/cloud as the workspaces they provision.

1. Allows you to use infrastructure-provided credentials (see Authentication below), which are typically easier to manage and have shorter lifetimes than credentials issued outside the cloud/cluster.
1. You don’t have to open any ingress ports on the clusters/clouds that host workspaces.
   - The external provisioner daemons dial out to Coder Server
   - Provisioner daemons run in the cluster, so you don’t need to expose cluster/cloud APIs externally
1. Each cloud/cluster is isolated, so a compromise of a provisioner daemon is limited to a single cluster.

### Authentication

Use a [Scoped Key](../../admin/provisioners.md#scoped-key-recommended) to authenticate the provisioner daemons with Coder. These keys can only be used to authenticate provisioner daemons (not other APIs on the Coder Server).

Store the keys securely and use environment variables to pass them to the provisioner daemon. If on Kubernetes, use a Kubernetes Secret to set the environment variable 

Tag provisioners with identifiers for the specific cluster/cloud. This allows your templates to target a specific cluster/cloud (for geographic proximity to the end user, or for specific features like GPUs or managed services). Scope your keys to organizations and the specific cluster/cloud using the same tags when creating the keys. This ensures that a compromised key will not allow an attacker to gain access to jobs for other clusters or organizations.

Provisioner daemons should have access only to cluster/cloud API credentials for the specific cluster/cloud they are for. This ensures that compromise of one Provisioner Daemon does not compromise all clusters/clouds.

Deploy the provisioner daemon into the cloud and leverage infrastructure-provided credentials. E.g.

1. [Service Account Tokens on Kubernetes](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/)
1. [IAM Roles for EC2 on AWS](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/iam-roles-for-amazon-ec2.html)
1. [Attached Service Accounts on Google Cloud](https://cloud.google.com/iam/docs/attach-service-accounts)

### Encryption in Transit

Enable TLS on Coder Server and ensure you use an `https://` URL to access the Coder Server.

See the Encryption in Transit subheading of the [Templates](#workspace-templates) section for more about encrypting cluster/cloud API calls.

### Encryption at Rest

Run provisioner daemons only on systems with full disk encryption enabled.

- Provisioner daemons temporarily persist terraform template files and resource state to disk. Either of these could contain sensitive information, including credentials. This temporary state is on disk only while actively building workspaces, but an attacker who compromises physical disks could successfully read this information if not encrypted.
- Provisioner daemons also store cached copies of Terraform provider binaries. These are generally not sensitive in terms of confidentiality, but it is important to maintain their integrity. An attacker that can modify these binaries could inject malicious code.

## Workspace Proxies

Workspace proxies authenticate end users and then proxy network traffic to workspaces. 

Coder takes care to ensure the user credentials processed by workspace proxies are scoped to application access, and do not grant full access to the Coder API on behalf of the user. Still, a fully compromised workspace proxy would be in a privileged position to phish unrestricted user credentials.

Workspace proxies have unrestricted access to establish encrypted tunnels to workspaces, and can access any port on any running workspace.

### Authentication

Securely store the workspace proxy token generated by [`coder wsproxy create`](../../admin/networking/workspace-proxies.md#step-1-create-the-proxy). Inject the token to the workspace proxy process via an environment variable, rather than via an argument. If on Kubernetes, use a Kubernetes Secret to set the environment variable.

### Encryption in Transit

Enable TLS on Coder Server and ensure you use an `https://` URL to access the Coder Server.

Communication to the proxied workspace applications is always encrypted with Wireguard. No action is required.

### Encryption at Rest

Workspace proxies persist no state locally. No action is required.

## Workspace Templates

Coder templates are executed on provisioner daemons, and can include arbitrary code via the [local-exec provisioner](https://developer.hashicorp.com/terraform/language/resources/provisioners/local-exec). Furthermore, Coder templates are designed to provision compute resources in one or more clusters/clouds, and template authors are generally in full control over code & scripts executed by the Coder agent in those compute resources.

This means that template admins effectively have remote code execution over any provisioner daemons in their organization, and within any cluster/cloud those provisioner daemons are credentialed to access.

Template admin is a powerful, highly-trusted role that you should not assign lightly. Instead of directly assigning the role to anyone who might need to edit a template, use [Git Ops](#git-ops) to allow users to author and edit templates.

## Secrets

Never include secrets like credentials directly in templates, including in `.tfvars` or other files uploaded with the template. Instead do one of the following (in roughly descending order of preference)

1. Store secrets in a central secrets manager then access the secrets at build time via a Terraform provider. E.g. [Vault](https://registry.terraform.io/providers/hashicorp/vault/latest/docs) or [AWS Secrets Manager](https://registry.terraform.io/providers/hashicorp/aws/latest/docs/resources/secretsmanager_secret).
1. Place secrets in `TF_VAR_*` environment variables provided to the relevant Provisioner Daemons, and access them via Terraform variables with `sensitive = true`.
1. Use Coder parameters to accept secrets from end users at build time.

Coder does not attempt to obscure the contents of template files from users authorized to view and edit templates, so secrets included directly could inadvertently appear on-screen while template authors do their work. 

Furthermore, template versions are persisted indefinitely in the PostgreSQL database, so if secrets are inadvertently included, they should be revoked as soon as practical. Pushing a new template version does not expunge them from the database. Contact support if you need assistance expunging any particularly sensitive data.

### Encryption in Transit

Always use encrypted transport to access any infrastructure APIs. Crucially, this protects confidentiality of the credentials used to access the APIs. Configuration of this depends on the specific Terraform providers in use and is beyond the scope of this document.

### Encryption at Rest

While your most privileged secrets should never be included in template files, they may inevitably contain confidential or sensitive data about your operations and/or infrastructure.

- Ensure that operators who write, review or modify Coder templates are working on laptops/workstations with full disk encryption, or do their work inside a Coder workspace with full disk encryption.
- Ensure [PostgreSQL](#postgresql) is encrypted at rest.
- Ensure any [source code repositories that store templates](#git-ops) are encrypted at rest and have appropriate access controls.

### Git Ops

Git Ops is the practice of using a Git repository as the source of truth for operational config, and reconciling the config in Git with operational systems each time the `main` (or, archaically, `master`) branch of the repository is updated.

As it relates to Coder templates, we recommend you store them in a single git repository, or a single repository per Coder organization, and use the [Coderd Terraform provider](https://registry.terraform.io/providers/coder/coderd/latest/docs/resources/template) to push changes from the main branch into Coder, using a CI/CD tool. This gives you an easily browsable, auditable history of template changes and who made them. (Coder audit logs establish who and when changes happen, but git repositories are particularly handy for analyzing exactly what changes to templates are made).

Use a Coder user account exclusively for the purpose of pushing template changes, and do not give any human users the credentials. This ensures any actions taken by the account correspond exactly to CI/CD actions from the repository, and allows you to avoid granting the template admin role widely in your organization.

You should use branch protection (e.g. on [GitHub](https://docs.github.com/en/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches), or equivalent for your source repository) to enforce code review of changes to templates. Code review increases the chance that someone will catch a potential security bug in your template.

These protections also mitigate the risk of a single trusted insider “going rogue” and acting unilaterally to maliciously modify Coder templates.

## Workspaces

The central purpose of Coder is to give end users access to managed compute in clusters/clouds designated by Coder’s operators (e.g. platform or developer experience teams). End users are granted shell access and from there can execute arbitrary commands.

This means that end users have remote code execution within clusters/clouds which host Coder workspaces.

It is important to limit Coder users to trusted insiders and/or take steps to constrain malicious activity that could be undertaken from a Coder workspace. Example constraints include:

- Network policy or segmentation
- Runtime protections on the workspace host (e.g. SELinux)
- Limiting privileges of the account or role assigned to the workspace (e.g. service account on Kubernetes, or IAM role on public clouds)
- Monitoring and/or auditing to look for suspicious activity (e.g. cryptomining, exfiltration)

## Outbound Network Access

Identify network assets like production systems or highly confidential data stores and configure the network to limit access from Coder workspaces.  If production systems or confidential data reside in the same cluster/cloud, use separate node pools and network boundaries. If extraordinary access is required, follow [Zero Trust](https://en.wikipedia.org/wiki/Zero_trust_security_model) principles:

- Authenticate the user and the workspace using strong cryptography
- Apply strict authorization controls
- Audit access in a tamper resistant secure store

Consider the network assets end users will need to do their job, and the level of trust the company has with them (e.g. in-house full-time employees vs. temporary contractors vs. 3rd party service providers). Restrict access as appropriate. A non-exclusive list of network assets to consider:

- Access to the public Internet
   - If end users will access the workspace over the public Internet, you must allow outbound access to establish the encrypted tunnels.
- Access to internal corporate networks
   - If end users will access the workspace over the corporate network, you must allow outbound access to establish the encrypted tunnels.
- Access to staging or production systems
- Access to confidential data (e.g. payment processing data, health records, personally identifiable information)
- Access to other clusters/clouds

## Inbound Network Access

Coder manages inbound network access to your workspaces via a set of Wireguard encrypted tunnels. These tunnels are established by sending outbound packets, so on stateful firewalls, disable inbound connections to workspaces to ensure inbound connections are exclusively via the encrypted tunnels.

## DERP

[DERP](https://tailscale.com/kb/1232/derp-servers) is a relay protocol developed by Tailscale. Coder Server and Workspace Proxies include a DERP service by default. Tailcale also runs a set of public DERP servers, globally distributed.

All DERP messages are end-to-end encrypted, so the DERP service only learns the (public) IP addresses of the participants. If you consider these addresses or the fact that pairs of them communicate over DERP to be sensitive, stick to the Coder-provided DERP services which run on your own infrastructure. If not, feel free to configure Tailscale DERP servers for global coverage.

#### STUN

[STUN](https://en.wikipedia.org/wiki/STUN) is an IETF standard protocol that allows network endpoints behind NAT to learn their public address / port mappings. It is an essential component of Coder’s networking to enable encrypted tunnels to be established without a relay for best performance.  Coder does not ship with a STUN service because it needs to be run directly connected to the network, not behind a reverse proxy or load balancer as Coder usually is.

STUN messages are not encrypted, but do not transmit any tunneled data, they simply query the public address / ports. As such, a STUN service learns the public address and port information (i.e. the address and port on the NAT device) of Coder workspaces and the end user device if STUN is configured. Unlike DERP, it doesn’t definitively learn about communicating pairs of IPs. If you consider the public IP and port information to be sensitive, do not use public STUN servers. You may choose not to configure any STUN servers, in which case most workspace traffic will need to be relayed via DERP. You may choose to deploy your own STUN servers, either on the public Internet, or on your corporate network and [configure Coder to use it](../../reference/cli/server.md#--derp-server-stun-addresses). If you do not consider the addresses and ports to be sensitive, we recommend using the default set of STUN servers operated by Google.

#### Workspace Apps

Coder workspace apps are a way to allow users to access web applications running in the workspace via the Coder Server or Workspace Proxy. [Disable workspace apps on sub-paths](../../reference/cli/server.md#--disable-path-apps) of the main Coder domain name, and [use a separate, wildcard domain name](../../admin/setup/index.md#wildcard-access-url) for forwarding. Because of the default [same-origin policy](https://en.wikipedia.org/wiki/Same-origin_policy) in browsers, serving web apps on the main Coder domain would allow those apps to send API requests to the Coder Server, authenticated as the logged in user without their explicit consent.

#### Port Sharing

Coder supports the option to allow users to designate specific network ports on their workspace as shared, which allows others to access those ports via the Coder Server. Consider restricting the maximum sharing level for workspaces, located in the template settings for the corresponding template.

### Encryption at Rest

Deploy Coder workspaces using full disk encryption for all volumes. This mitigates attempts to recover sensitive data in the workspace by attackers who gain physical access to the disk(s).
