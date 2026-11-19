---
title: Sandbox agents
---

The term "sandboxing" has become popular in the era of agentic coding to describe secure, isolated environments for AI agents.
Common traits of a sandbox include:

- **Kernel isolation**: agents run inside their own kernel, which reduces the risk of an escape to the host.
- **Network isolation**: agents run with no (or limited) network access, which reduces the risk of data loss, a compromised supply chain, or prompt injection.
- **Secrets isolation**: agents run with isolated or scoped-down secrets and identity, compared to the user, which reduces the blast radius.

Additionally, sandboxes are often seen as faster and lighter weight than traditional compute, quickly provisioning for agentic tasks.
Coder workspaces can have every trait of a sandbox, including fast startup, and Coder supports the wider sandbox ecosystem.
Each trait is a building block you add to a template based on your infrastructure and requirements, giving you control over where and how agents run.

This page is a guide for Coder administrators on how to build and configure Coder templates and the underlying infrastructure to sandbox AI agents, according to the previous definitions.
Because Coder is flexible and infrastructure-agnostic, this page outlines several ways to achieve each trait, and the tradeoffs of each.

| Trait                                   | Options                                                       | Main tradeoff                                                          |
|-----------------------------------------|---------------------------------------------------------------|------------------------------------------------------------------------|
| [Kernel isolation](#kernel-isolation)   | VMs, gVisor, Kata Containers, Nitro Sandboxes                 | Stronger isolation needs node support that managed node pools can lack |
| [Network isolation](#network-isolation) | NetworkPolicy, egress proxy, OpenShell, Cilium                | Finer rules need more components to run                                |
| [Secrets isolation](#secrets-isolation) | API key scopes, AI Gateway, service accounts, dynamic secrets | Less access means more setup for each credential                       |
| [Fast startup](#fast-startup)           | Prebuilt workspaces, faster templates, warm pools             | Ready capacity costs idle resources                                    |

## Start from a registry template

These templates in the [Coder Registry](https://registry.coder.com/templates) support all three traits, plus fast startup.
Each one is a starting point that you adapt to your cluster.

| Template                                                                                               | Kernel isolation        | Network isolation                       | Secrets isolation       | Fast startup                          |
|--------------------------------------------------------------------------------------------------------|-------------------------|-----------------------------------------|-------------------------|---------------------------------------|
| [agent-sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-agent-sandbox) | gVisor or Kata          | NetworkPolicy                           | Scoped AI Gateway token | Prebuilds or agent-sandbox warm pools |
| [Plain Sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-sandboxed)     | Optional gVisor or Kata | Squid proxy by host name                | Scoped AI Gateway token | Prebuilds                             |
| [OpenShell (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-openshell)         | Optional Kata           | OpenShell rules by host name and binary | Scoped AI Gateway token | Prebuilds                             |

> [!TIP]
> [agent-sandbox](https://agent-sandbox.sigs.k8s.io/) is a Kubernetes Special Interest Group (SIG) project that adds primitives for agent workloads.
> A cluster administrator sets the runtime, image, and network rules once in a `SandboxTemplate`, and a `SandboxWarmPool` keeps booted sandboxes ready.
> To use it with Coder, start from the [agent-sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-agent-sandbox) registry template.

## Sandbox coverage by access method

The sandbox is the workspace that your template builds.
Kernel, network, and secrets isolation apply to every process in that workspace, and to nothing outside it.

You can use one template to sandbox every way agents access Coder, but the amount of control varies by access method, because each one runs a different share of the agent in the workspace:

| Access method                                                                                                                                                       | Inside the sandbox                                                                                                 | Outside the sandbox                                                                                                                                                        | What the sandbox controls                                      |
|---------------------------------------------------------------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------|
| CLI Agents, such as the [Claude Code](https://registry.coder.com/modules/coder/claude-code) or [Codex](https://registry.coder.com/modules/coder-labs/codex) modules | The agent loop, tool calls, commands, Model Context Protocol (MCP) servers, hooks, and subagents                   | Tools that the model provider hosts, such as web search                                                                                                                    | Tool calls, commands, servers, model requests, and credentials |
| [Coder Agents](../../coder-agents/index.md)                                                                                                                         | Tool calls, commands, and servers from a workspace [`.mcp.json`](../../coder-agents/bring-your-own-mcp-servers.md) | The agent loop, model credentials, [admin-configured MCP servers](../../coder-agents/platform-controls/mcp-servers.md), and control plane tools such as `create_workspace` | Tool calls, commands, and servers in the workspace             |
| [Agent Relay](../../agent-relay/index.md)                                                                                                                           | Tool calls, commands, and servers that the worker starts                                                           | The agent loop and the provider's own tools, in the provider's cloud                                                                                                       | Tool calls, commands, and servers in the workspace             |

To control what runs outside the sandbox, use the settings of the system that runs it, such as MCP server availability in Coder Agents.

## Define sandbox profiles as parameters

One template can offer several sandbox profiles, such as "Fully isolated" and "Internal GitHub".
Each profile is a fixed set of rules: the RuntimeClass, the hosts that the proxy allows, and the token scopes.

Make the profile an immutable parameter, with `mutable = false`.
Coder rejects any later build that changes an immutable parameter, so a workspace keeps the profile it was created with until you delete it.
Neither the agent nor the user can loosen the sandbox with a workspace update or restart.

Add one [preset](./parameters/workspace-presets.md) for each profile, and make the strictest one the default.
A preset sets the profile when the workspace is created, and can keep a pool of [prebuilt workspaces](./prebuilt-workspaces.md) for each profile.
Because the preset sets every immutable parameter, a claimed prebuild never changes its profile.

```tf
locals {
  profiles = {
    isolated = { display_name = "Fully isolated", runtime = "gvisor", hosts = [] }
    github   = { display_name = "Internal GitHub", runtime = "gvisor", hosts = ["github.example.com"] }
    develop  = { display_name = "GitHub and packages", runtime = "kata-qemu", hosts = ["github.example.com", "artifactory.example.com"] }
  }
  profile = local.profiles[data.coder_parameter.sandbox_profile.value]
}

data "coder_parameter" "sandbox_profile" {
  name         = "sandbox_profile"
  display_name = "Sandbox profile"
  description  = "What the workspace can reach. You cannot change it after create."
  type         = "string"
  form_type    = "dropdown"
  default      = "isolated"
  mutable      = false

  dynamic "option" {
    for_each = local.profiles
    content {
      name  = option.value.display_name
      value = option.key
    }
  }
}

data "coder_workspace_preset" "profile" {
  for_each   = local.profiles
  name       = each.value.display_name
  default    = each.key == "isolated"
  parameters = { sandbox_profile = each.key }
}
```

Then use `local.profile.runtime` as the Pod's `runtime_class_name`, and `local.profile.hosts` as the proxy's allowlist.
The [Plain Sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-sandboxed) template uses this pattern for its network profiles and kernel isolation.

Coder Agents chooses the preset and parameter values when it calls `create_workspace`, so a parameter alone lets the agent pick a looser profile.
To decide the profile for the agent:

- **Limit options by group**: with [identity-aware parameters](./parameters/identity-aware-parameters.md), show a profile only to members of a group, such as `data.coder_workspace_owner.me.groups`.
  Coder rejects values that aren't in the owner's options.
  If prebuilds use a limited profile, add the `prebuilds` user to the group.
- **Use one template for each profile**: give agents only the strict templates.
  On every other template, turn off **Allow Coder Agents to create workspaces using this template**, as in [Restrict available templates](../../coder-agents/platform-controls/template-optimization.md#restrict-available-templates).
  Developers can still create workspaces from the other templates.

Template names, descriptions, and organization instructions help the agent pick the right profile, but they don't enforce it.

## Kernel isolation

Containers on a node share its kernel, so a kernel exploit can escape to the node and its other workloads.
Kernel isolation gives each workspace its own kernel, or a kernel in user space, so the exploit stays inside.

| Method                                                                              | How it works                                                                                                                       | Tradeoffs                                                                                                                                                                     |
|-------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| VM-based templates                                                                  | Each workspace is its own virtual machine, such as the [AWS Linux](https://registry.coder.com/templates/coder/aws-linux) template. | More expensive, and slower to start than a container.                                                                                                                         |
| [gVisor](https://gvisor.dev/docs/user_guide/containerd/quick_start/)                | A kernel that runs in user space and handles the workspace's system calls.                                                         | Slower, especially for file-heavy work. It still uses some host kernel features, and some Linux features don't work.                                                          |
| [Kata Containers](https://katacontainers.io/)                                       | Each Pod runs in a small VM with its own kernel, with a hypervisor such as QEMU or Cloud Hypervisor.                               | Needs bare metal or nested virtualization, which many managed nodes don't have. Uses more memory for each Pod.                                                                |
| [Nitro Sandboxes](https://docs.aws.amazon.com/eks/latest/userguide/sandboxing.html) | Each Pod runs in its own Firecracker VM on the AWS Nitro hypervisor, with the `isolated-sandbox` RuntimeClass.                     | Amazon Elastic Kubernetes Service (EKS) only. Built into EKS Auto Mode; managed node groups need a custom Amazon Machine Image (AMI). No GPUs, and a fixed size for each Pod. |

[Sysbox](./docker-in-workspaces.md#sysbox-container-runtime) isn't kernel isolation.
It runs Docker and systemd in the workspace without a privileged container, by mapping root in the workspace to an unprivileged user on the node, but every system call still goes to the node's kernel.
Use it when agents need Docker, and add gVisor or Kata if they also need kernel isolation.
For what its user namespaces cover, refer to [Sysbox security](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/security.md).

On Kubernetes, install gVisor or Kata on the nodes, and add a [RuntimeClass](https://kubernetes.io/docs/concepts/containers/runtime-class/) for it to the cluster.
EKS Auto Mode already has the `isolated-sandbox` RuntimeClass for Nitro Sandboxes.
To use one, set `runtime_class_name` on the workspace Pod in your template:

```tf
resource "kubernetes_pod_v1" "workspace" {
  # ...
  spec {
    runtime_class_name = "gvisor"
    # ...
  }
}
```

The RuntimeClass name is whatever the cluster administrator created, such as `gvisor`, `kata-qemu`, or `isolated-sandbox`.
To let users pick a runtime, set `runtime_class_name` from a [parameter](./parameters/index.md).
To run sandboxed Pods on their own nodes, add a node selector and a [toleration](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) for a tainted node pool.

[Workspace process logging](./process-logging.md) runs a privileged eBPF sidecar in the workspace Pod.
With gVisor, Kata, or Nitro Sandboxes, that sidecar runs inside the sandbox too, so test process logging with your runtime before you rely on it.

Examples and references:

- [agent-sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-agent-sandbox) template: a cluster administrator sets gVisor or Kata once in a `SandboxTemplate`, and workspaces can only claim sandboxes from it.
- [Plain Sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-sandboxed) template: list the gVisor or Kata RuntimeClasses on your nodes to add kernel isolation to plain Pods.
- [AWS Linux](https://registry.coder.com/templates/coder/aws-linux) template: a VM for each workspace.
- [GKE Sandbox](https://cloud.google.com/kubernetes-engine/docs/concepts/sandbox-pods): gVisor node pools on Google Kubernetes Engine (GKE).
- [Pod Sandboxing on Azure Kubernetes Service (AKS)](https://learn.microsoft.com/en-us/azure/aks/use-pod-sandboxing): Kata node pools on AKS.
- [Kata Containers installation](https://github.com/kata-containers/kata-containers/blob/main/docs/install/README.md): install options for other clusters, including [Firecracker](https://firecracker-microvm.github.io/) as the hypervisor.
- [gVisor security model](https://gvisor.dev/docs/architecture_guide/security/): what gVisor protects against, and what it doesn't.
- [Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/): block privileged Pods in the namespace that runs agent workspaces.

## Network isolation

Network isolation works as a firewall for the agent.
It limits where the agent can send data, so a prompt injection can't send your code or credentials to an attacker's host.
It also limits where the agent can download from, which reduces the risk of a compromised package or a page with injected instructions.

The workspace always needs to reach the Coder server.
The Coder workspace agent relays connections through it, so the workspace needs no inbound access.
For the connections Coder uses, refer to [Outbound network access](../../admin/security/security-best-practices.md#outbound-network-access).

| Method                                                                                                | Rules by                                       | Applies to        | Tradeoffs                                                                                                                      |
|-------------------------------------------------------------------------------------------------------|------------------------------------------------|-------------------|--------------------------------------------------------------------------------------------------------------------------------|
| [Kubernetes NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/) | IP address and port                            | The whole Pod     | Built in, but IP-based, so services that change addresses, such as GitHub, are hard to allow.                                  |
| Egress proxy, such as [Squid](https://www.squid-cache.org/)                                           | Host name                                      | The whole Pod     | Another component to run and update, as a separate Pod or a sidecar.                                                           |
| [NVIDIA OpenShell](https://docs.nvidia.com/openshell/latest/about/overview)                           | Host name and the binary that connects         | The whole sandbox | Nested setup: a cluster gateway, plus a launcher Pod and a supervisor Pod for each workspace. Slower to stop than a plain Pod. |
| [Cilium](https://docs.cilium.io/en/stable/security/dns/)                                              | Host name, IP address, port, and HTTP requests | The whole Pod     | Cilium must be the cluster's network plugin.                                                                                   |

> [!WARNING]
> Kubernetes accepts a NetworkPolicy even when the network plugin doesn't enforce it.
> Test each rule from inside a workspace before you run an agent in it.

An egress proxy runs in one of two places:

- **Separate proxy Pod**: the proxy runs in its own Pod, either one for each workspace or one shared by all workspaces.
  A NetworkPolicy lets the workspace Pod reach only Coder and the proxy Pod, so the agent can't go around the proxy.
  The [Plain Sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-sandboxed) template uses this setup, with a separate Squid Pod for each workspace.
- **Sidecar**: the proxy runs as a second container in the workspace Pod.
  It's simpler to deploy, but the two containers share one network, and a NetworkPolicy can't tell their traffic apart.
  To stop the agent from going around the proxy, send all of the Pod's outbound traffic to the proxy, except the proxy's own, such as with `iptables` rules in an init container.

Rules by host name still allow every path on an allowed host.
If an allowed host stores user content, such as a code host or a file share, an agent can still send data to it.

A rule that applies to the whole Pod is harder to bypass than one that wraps a process, because a command the agent starts in another way still goes through it.
For more detail, refer to [Why network boundaries are more effective than process-level controls](../../coder-agents/platform-controls/template-optimization.md#why-network-boundaries-are-more-effective-than-process-level-controls).

Every method in this section runs in the cluster, so a mistake in a template or cluster setting can open it.
For a layer that holds anyway, have your security team run a firewall outside the cluster, such as [AWS Network Firewall](https://docs.aws.amazon.com/network-firewall/latest/developerguide/what-is-aws-network-firewall.html) with an allowlist of TLS server names, [Route 53 Resolver DNS Firewall](https://docs.aws.amazon.com/Route53/latest/DeveloperGuide/resolver-dns-firewall.html), or an [S3 gateway endpoint](https://docs.aws.amazon.com/vpc/latest/privatelink/vpc-endpoints-s3.html) policy.
Limit DNS too, because an agent that can resolve any name can send data out in DNS lookups.

### Example rules

Start with a rule that matches what the agent does.
The examples use `github.example.com` for your internal GitHub, such as GitHub Enterprise Server, and `artifactory.example.com` for your internal artifact store, such as JFrog Artifactory or Sonatype Nexus.
Replace them with your own hosts.

| Use case                                                                   | Allow                                                      | Block                                                             | Fits                                          |
|----------------------------------------------------------------------------|------------------------------------------------------------|-------------------------------------------------------------------|-----------------------------------------------|
| Send all model traffic through AI Gateway                                  | Everything else                                            | Model providers, such as `api.anthropic.com` and `api.openai.com` | Egress proxy, Cilium, or a corporate firewall |
| A known workflow, such as a docs review bot that comments on pull requests | Coder, `github.example.com`, and `artifactory.example.com` | Everything else                                                   | Egress proxy, OpenShell, or Cilium            |
| A research task on sensitive data                                          | Coder and `artifactory.example.com`                        | Everything else                                                   | NetworkPolicy                                 |

The first rule makes sure that AI Gateway can monitor and audit every model request.
If your network already blocks model providers, you don't need it in the template.
The second rule fits any workflow where you know what the agent needs.
An internal GitHub keeps the agent's traffic in your network, which is stricter than an allow rule for all of `github.com`.
OpenShell can also limit `github.example.com` to the `git` binary.
The third rule turns off the internet, which suits long tasks that don't need it.
The agent can still install packages from the artifact store.

> [!NOTE]
> An artifact store that proxies public registries, such as npm or PyPI, can send requests to the internet for the agent.
> For the strictest rules, allow only repositories that serve packages your team already approved.

Examples and references:

- [Plain Sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-sandboxed) template: a NetworkPolicy and a Squid proxy that allow host names, such as an internal GitHub or artifact store.
- [OpenShell (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-openshell) template: rules by host name and binary, and Landlock file limits.
- [agent-sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-agent-sandbox) template: a NetworkPolicy firewall, with ClusterNetworkPolicy or Cilium for more advanced rules.
- [Network isolation for Coder Agents](../../coder-agents/platform-controls/agent-isolation.md): a template that runs the agent in its own Pod with a NetworkPolicy.
- [Squid access controls](https://www.squid-cache.org/Doc/config/acl/): allowlists by host name for an egress proxy.
- [Cilium policy language](https://docs.cilium.io/en/stable/security/policy/language/): rules by DNS name, port, and HTTP request.
- [OpenShell source](https://github.com/NVIDIA/OpenShell): the policy format and the sandbox image.

## Secrets isolation

An agent can use any credential it can read, and a prompt injection can tell it to send that credential somewhere.
Secrets isolation keeps credentials that the task doesn't need out of the workspace, and gives the rest the smallest scope that works.

Every process that the Coder workspace agent starts, such as a terminal, an app, a startup script, or an AI agent, gets the workspace agent token in `CODER_AGENT_TOKEN`.
An AI agent that reads this token can use it to get:

- The workspace owner's Git SSH key.
- The owner's tokens for each [external auth](../../admin/external-auth.md) provider that the owner has linked.
- The [user secrets](../../workspaces/user-secrets.md) and [workspace secrets](../../admin/security/secrets.md#workspace-secrets) that Coder injects into the workspace.

So the identity that owns the workspace sets the blast radius.
To limit what an agent can read:

| Method                                                                | How it works                                                                                                                        | Tradeoffs                                                                                                                                                                       |
|-----------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| [API key scopes](../../admin/users/sessions-tokens.md#api-key-scopes) | Give the agent a token with only the scopes it needs, instead of the owner's session token.                                         | Available in Coder. You pick the scopes for each task, and a scope that's too narrow fails the task.                                                                            |
| [AI Gateway](../../ai-governance/ai-gateway/index.md)                 | The agent calls model providers through Coder with a Coder token, so the provider API keys stay on the server.                      | Coder Premium. The agent still needs a Coder token to authenticate, so pair it with a scoped token or a service account.                                                        |
| [Service accounts](../../admin/users/headless-auth.md)                | Run autonomous agents as a headless user with no linked external auth, no personal secrets, and only the template access they need. | Coder Premium. Work isn't tied to the person who asked for it, so set a [Git identity](../../coder-agents/platform-controls/template-optimization.md#git-identity) for commits. |
| [Dynamic secrets](../../admin/security/secrets.md#dynamic-secrets)    | Create a short-lived credential or cloud identity for each workspace in the template, tied to the workspace lifecycle.              | A leaked credential expires on its own. Needs a secrets manager or cloud identity setup for each provider.                                                                      |

### Scope the workspace agent token

Set `api_key_scope = "no_user_data"` on the `coder_agent` so its token can't fetch the owner's Git SSH key or external auth tokens:

```tf
resource "coder_agent" "main" {
  os            = "linux"
  arch          = "amd64"
  api_key_scope = "no_user_data"
}
```

The tradeoff is that Git over SSH and Git credentials from external auth stop working in the workspace, so give the agent its own Git credential, such as a [dynamic secret](../../admin/security/secrets.md#dynamic-secrets).
The scope doesn't affect user secrets and workspace secrets, which Coder still injects.
To keep those out, run the agent as a [service account](../../admin/users/headless-auth.md) that has none.

### Scope the AI Gateway token

`data.coder_workspace_owner.me.session_token` has the owner's full access to the Coder API, and the [AI Gateway client examples](../../ai-governance/ai-gateway/clients/index.md) inject it so the agent can authenticate.
Instead, use the session token once at build time to create a token with only the `user:read_personal` scope, which AI Gateway accepts, and give the workspace that token:

```tf
data "coder_workspace" "me" {}
data "coder_workspace_owner" "me" {}

data "http" "ai_gateway_token" {
  count  = data.coder_workspace.me.is_prebuild ? 0 : data.coder_workspace.me.start_count
  url    = "${data.coder_workspace.me.access_url}/api/v2/users/me/keys/tokens"
  method = "POST"
  request_headers = {
    "Coder-Session-Token" = data.coder_workspace_owner.me.session_token
    "Content-Type"        = "application/json"
  }
  request_body = jsonencode({
    token_name = "ai-gateway-${substr(data.coder_workspace.me.id, 0, 8)}-${formatdate("YYYYMMDDhhmmss", timestamp())}"
    scopes     = ["user:read_personal"]
    # Nanoseconds. Keep it longer than the template's autostop.
    # lifetime = 8 * 3600 * 1000000000
  })

  lifecycle {
    postcondition {
      condition     = self.status_code == 201
      error_message = "Coder did not create the AI Gateway token."
    }
  }
}

resource "coder_env" "anthropic_base_url" {
  count    = data.coder_workspace.me.start_count
  agent_id = coder_agent.main.id
  name     = "ANTHROPIC_BASE_URL"
  value    = "${data.coder_workspace.me.access_url}/api/v2/ai-gateway/anthropic"
}

resource "coder_env" "anthropic_auth_token" {
  count    = data.coder_workspace.me.is_prebuild ? 0 : data.coder_workspace.me.start_count
  agent_id = coder_agent.main.id
  name     = "ANTHROPIC_AUTH_TOKEN"
  value    = sensitive(jsondecode(data.http.ai_gateway_token[0].response_body).key)
}
```

Each start creates a new token.
Coder can't revoke it at stop, so it stays valid until it expires, after the deployment's default token lifetime (7&nbsp;days by default).
To shorten that, set `lifetime`, but keep it longer than the template's autostop so the token doesn't expire in a running workspace.
The `count` skips prebuilds, because Coder doesn't create tokens for the `prebuilds` user, and the claim build creates the token instead.
For a full version, refer to the [Plain Sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-sandboxed) template.

The scoped token helps only if the workspace has no other copy of the session token.
Coder doesn't set `CODER_SESSION_TOKEN` in a workspace by itself.
The [coder-login module](https://registry.coder.com/modules/coder/coder-login) sets it, so the `coder` CLI in the workspace acts as the owner.
Other modules also inject the session token when you turn on their AI Gateway option, such as `ANTHROPIC_AUTH_TOKEN` in the [Claude Code module](https://registry.coder.com/modules/coder/claude-code) with `enable_ai_gateway = true`.
To keep the session token out of the workspace:

- Remove the coder-login module.
- Turn off the AI Gateway option in agent modules, and set the scoped token yourself, as in the example.
- Search the template for other references to `data.coder_workspace_owner.me.session_token`, and keep only those used at build time, such as the `http` data source in the example.

Then run `env | grep -i token` in a workspace to confirm that only the scoped token and `CODER_AGENT_TOKEN` are there.

### Provision just-in-time credentials

Create a short-lived credential for each workspace start, instead of a long-lived key in the image or a user secret.
This example reads AWS Security Token Service (STS) credentials from the [Vault AWS secrets engine](https://developer.hashicorp.com/vault/docs/secrets/aws) at build time, with a one-hour lifetime:

```tf
data "vault_aws_access_credentials" "agent" {
  count   = data.coder_workspace.me.start_count
  backend = "aws"
  role    = "coder-agent"
  type    = "sts"
  ttl     = "1h"
}

resource "coder_env" "aws_access_key_id" {
  count    = data.coder_workspace.me.start_count
  agent_id = coder_agent.main.id
  name     = "AWS_ACCESS_KEY_ID"
  value    = data.vault_aws_access_credentials.agent[0].access_key
}

resource "coder_env" "aws_secret_access_key" {
  count    = data.coder_workspace.me.start_count
  agent_id = coder_agent.main.id
  name     = "AWS_SECRET_ACCESS_KEY"
  value    = sensitive(data.vault_aws_access_credentials.agent[0].secret_key)
}

resource "coder_env" "aws_session_token" {
  count    = data.coder_workspace.me.start_count
  agent_id = coder_agent.main.id
  name     = "AWS_SESSION_TOKEN"
  value    = sensitive(data.vault_aws_access_credentials.agent[0].security_token)
}
```

The Vault role sets what the credential can do, so give it only the AWS permissions that the agent's tasks need.
A leaked credential stops working when its lifetime ends, and a restart issues a new one.
The credential is also stored in the workspace's Terraform state, so keep the lifetime short.

On EKS, give each workspace its own service account and [Pod Identity](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html) association, such as one named after the workspace ID.
Workspaces can then share one AWS Identity and Access Management (IAM) role, and its policy can limit each workspace to its own data with the [`${aws:PrincipalTag/kubernetes-service-account}`](https://docs.aws.amazon.com/eks/latest/userguide/pod-id-abac.html) session tag, such as an S3 prefix for each workspace.
Credentials copied out of a workspace work from anywhere until they expire, so add an `aws:SourceVpce` or `aws:SourceIp` [condition](https://docs.aws.amazon.com/IAM/latest/UserGuide/reference_policies_condition-keys.html) to tie them to your network.

Examples and references:

- [Plain Sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-sandboxed) template: the token from [Scope the AI Gateway token](#scope-the-ai-gateway-token), with prebuild handling.
- [AI Gateway Proxy module](https://registry.coder.com/modules/coder/aibridge-proxy): route an agent CLI's model traffic through AI Gateway, so provider API keys stay out of the workspace.
- [Vault JWT module](https://registry.coder.com/modules/coder/vault-jwt): authenticate to Vault with the user's OpenID Connect (OIDC) token instead of a static secret in the workspace.
- [Scope permissions and credentials](../../coder-agents/platform-controls/template-optimization.md#scope-permissions-and-credentials) and [Git identity](../../coder-agents/platform-controls/template-optimization.md#git-identity): credentials and commit identity for agent templates.
- [API key scopes reference](../../reference/api-key-scopes.md): each scope and the permissions it grants.
- [External auth in templates](./external-auth.md): which providers a template requests, and when the owner links them.
- [AI Gateway authentication](../../ai-governance/ai-gateway/auth.md): how clients, gateway keys, and provider keys differ.
- [Amazon EKS Pod Identity](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html), [GKE Workload Identity Federation](https://cloud.google.com/kubernetes-engine/docs/concepts/workload-identity), and [AKS Workload Identity](https://learn.microsoft.com/en-us/azure/aks/workload-identity-overview): give a workspace Pod a cloud identity without a static key.

## Fast startup

Fast startup isn't isolation, but it decides whether people use the sandbox.
An agent that starts a new workspace for each task creates many more workspaces than a person does.
If the sandbox takes minutes to start, users run the agent on their own machine instead, outside every control on this page.

To see a Kubernetes template go from 98&nbsp;seconds to 1.8&nbsp;seconds, watch [Improving Coder Workspace Startup Times](https://www.youtube.com/watch?v=-xcVJQWVCxs&t=514s).

| Method                                                         | How it works                                                                                       | Tradeoffs                                                                                 |
|----------------------------------------------------------------|----------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------|
| [Prebuilt workspaces](./prebuilt-workspaces.md)                | Coder keeps a pool of ready workspaces for each preset, and a user claims one at create.           | Coder Premium. Idle workspaces cost compute, and a slow claim can cancel out the gain.    |
| [Speed up templates](../../workspaces/speed-up-templates.md)   | Find slow steps with the build timeline, and move tools into the image instead of startup scripts. | Available in Coder, with no idle cost. A larger image takes longer to pull on a new node. |
| [agent-sandbox](https://agent-sandbox.sigs.k8s.io/) warm pools | A `SandboxWarmPool` keeps booted sandboxes ready in the cluster.                                   | Another controller to run in the cluster, and idle sandboxes cost compute.                |

A claim changes the workspace owner, so a resource that uses the owner's name in an immutable field is replaced at claim, and the claim is slow.
To keep the claim fast, refer to [Prevent resource replacement](./prebuilt-workspaces.md#prevent-resource-replacement).

With [sandbox profiles](#define-sandbox-profiles-as-parameters), each profile's preset keeps its own pool of prebuilt workspaces.
Because the profile is immutable and the preset sets it, the claim keeps the profile the prebuild was built with.
Size each pool for how often agents use that profile, and skip the pool for profiles that are rarely used.

Examples and references:

- [agent-sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-agent-sandbox) template: prebuilds or agent-sandbox warm pools.
- [Plain Sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-sandboxed) and [OpenShell (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-openshell) templates: prebuilds, with a speed tuning guide in each README.
- [Pre-warm Kubernetes nodes](../../workspaces/speed-up-templates.md#pre-warm-kubernetes-nodes) and [build tools into the image](../../workspaces/speed-up-templates.md#build-tools-into-the-image): keep nodes ready during working hours, and skip downloads at start.
- [Prebuilt workspace scheduling](./prebuilt-workspaces.md#scheduling): scale the pool up during work hours and down at night.
- [Manage resource quotas](./prebuilt-workspaces.md#manage-resource-quotas): account for idle prebuilds in user and group quotas.
- [Pre-install tools and dependencies](../../coder-agents/platform-controls/template-optimization.md#pre-install-tools-and-dependencies): bake the agent CLI and language toolchains into the image.
- [Agent Sandbox on GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/agent-sandbox): Google's guide to run the agent-sandbox controller on GKE with gVisor.

## Audit agent activity

A sandbox limits what an agent can do.
Logs show what it did, so you can tell whether the limits worked.

| Method                                                       | Provided by   | What it records                                             |
|--------------------------------------------------------------|---------------|-------------------------------------------------------------|
| [AI Gateway audit](../../ai-governance/ai-gateway/audit.md)  | Coder Premium | Prompts, tool calls, and token use for each model request.  |
| [Workspace process logging](./process-logging.md)            | Coder Premium | Each process started in the workspace container, with eBPF. |
| [Connection logs](../../admin/monitoring/connection-logs.md) | Coder Premium | SSH, app, and port-forward sessions into the workspace.     |
| [Audit logs](../../admin/security/audit-logs.md)             | Coder Premium | Changes to templates, workspaces, users, and tokens.        |

## Learn more

- [Docker in workspaces](./docker-in-workspaces.md)
- [Template optimization for Coder Agents](../../coder-agents/platform-controls/template-optimization.md)
- [AI Gateway](../../ai-governance/ai-gateway/index.md)
- [Agent Relay](../../agent-relay/index.md)
- [Security best practices](../../admin/security/security-best-practices.md)
- [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox)
