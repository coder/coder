---
title: Sandboxing agents
---

An AI agent runs commands, installs packages, and reads files without a person checking each step.
A prompt injection in a README, an issue, or a web page can change what the agent does.
An agent that can read private data, read untrusted content, and send data out is open to the [lethal trifecta](https://simonwillison.net/2025/Jun/16/the-lethal-trifecta/) of data theft, and the [Open Worldwide Application Security Project (OWASP) Top 10 for LLM applications](https://genai.owasp.org/llmrisk/llm01-prompt-injection/) ranks prompt injection as the first risk.
An agent sandbox is a workspace that limits what the agent can reach, so a mistake or an attack stays inside that workspace.

This page is for template authors and platform engineers who build workspaces for agents.
The same template works for each way an agent runs in Coder:

- An agent CLI in the workspace, such as the [Claude Code](https://registry.coder.com/modules/coder/claude-code) or [Codex](https://registry.coder.com/modules/coder-labs/codex) registry modules.
- [Coder Agents](../../coder-agents/index.md), which runs the agent loop in the control plane and its tools in a workspace.
- [Agent Relay](../../agent-relay/index.md), which runs a cloud agent's tool calls in a self-hosted workspace, such as the [kubernetes-agent-relay](https://registry.coder.com/templates/coder/kubernetes-agent-relay) template.

The page describes the four properties an agent sandbox can combine, and which ones Coder provides and which come from a runtime, network plugin, or proxy you add to the template.

| Property                                | What it limits                                      | Coder provides                              | You add with the template                              |
|-----------------------------------------|-----------------------------------------------------|---------------------------------------------|--------------------------------------------------------|
| [Fast startup](#fast-startup)           | Nothing, but users skip a slow sandbox              | Prebuilt workspaces and the build timeline  | A cluster warm pool, such as agent-sandbox             |
| [Kernel isolation](#kernel-isolation)   | Access to the host kernel and other workloads on it | VM-based templates                          | A container runtime such as gVisor, Kata, or Sysbox    |
| [Network isolation](#network-isolation) | The hosts and ports the agent can connect to        | Agent Firewall                              | A NetworkPolicy, an egress proxy, OpenShell, or Cilium |
| [Secret isolation](#secret-isolation)   | The credentials that the agent can read             | Scoped tokens, AI Gateway, service accounts | Short-lived cloud credentials for each workspace       |

Each property is independent, so you can pick the ones your agents need.
A template applies them per workspace, and [parameters](./parameters/index.md) or [presets](./parameters/workspace-presets.md) let users or groups pick a profile, such as a stricter network for one team.
To keep agent workspaces apart from the ones people use, [create dedicated agent templates](../../coder-agents/platform-controls/template-optimization.md#create-dedicated-agent-templates) and [restrict which templates agents can use](../../coder-agents/platform-controls/template-optimization.md#restrict-available-templates).

## Fast startup

Agents often start a new workspace for each task, so they create many more workspaces than people do.
If the sandbox takes minutes to start, users run the agent on their own machine instead.

| Method                                                         | Provided by        | Description                                                                                        |
|----------------------------------------------------------------|--------------------|----------------------------------------------------------------------------------------------------|
| [Prebuilt workspaces](./prebuilt-workspaces.md)                | Coder Premium      | Coder keeps a pool of ready workspaces for each preset, and a user claims one at create.           |
| [Speed up templates](../../workspaces/speed-up-templates.md)   | Coder              | Find slow steps with the build timeline, and move tools into the image instead of startup scripts. |
| [agent-sandbox](https://agent-sandbox.sigs.k8s.io/) warm pools | Kubernetes project | A `SandboxWarmPool` keeps booted sandboxes ready in the cluster. It doesn't need Coder Premium.    |

A claim changes the workspace owner, so a resource that uses the owner's name in an immutable field is replaced at claim, and the claim is slow.
To keep the claim fast, refer to [Prevent resource replacement](./prebuilt-workspaces.md#prevent-resource-replacement).

Examples and references:

- [Prebuilt workspace scheduling](./prebuilt-workspaces.md#scheduling): scale the pool up during work hours and down at night.
- [Manage resource quotas](./prebuilt-workspaces.md#manage-resource-quotas): account for idle prebuilds in user and group quotas.
- [Pre-install tools and dependencies](../../coder-agents/platform-controls/template-optimization.md#pre-install-tools-and-dependencies): bake the agent CLI and language toolchains into the image.
- [Agent Sandbox on GKE](https://cloud.google.com/kubernetes-engine/docs/how-to/agent-sandbox): Google's guide to run the agent-sandbox controller on Google Kubernetes Engine (GKE) with gVisor.

## Kernel isolation

A standard container shares the node's kernel with every other container on the node.
If an agent runs an exploit for a kernel bug, it can escape the container and reach the node and the other workloads on it.
Kernel isolation gives each agent workspace its own kernel, or a kernel that runs in user space.

| Method                                                               | Provided by          | Description                                                                                                                        | Requirements                                                              |
|----------------------------------------------------------------------|----------------------|------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------|
| VM-based templates                                                   | Coder and your cloud | Each workspace is its own virtual machine, such as the [AWS Linux](https://registry.coder.com/templates/coder/aws-linux) template. | Slower to start than a container.                                         |
| [gVisor](https://gvisor.dev/docs/user_guide/containerd/quick_start/) | Google, open source  | A kernel that runs in user space and handles the workspace's system calls.                                                         | Any Linux node. Slows file-heavy work, such as a large `git clone`.       |
| [Kata Containers](https://katacontainers.io/)                        | Open source          | Each Pod runs in a small VM with its own kernel, with a hypervisor such as QEMU or Cloud Hypervisor.                               | Bare metal nodes, or VMs with nested virtualization.                      |
| [Sysbox](./docker-in-workspaces.md#sysbox-container-runtime)         | Open source          | Isolates the workspace with Linux user namespaces, and runs Docker and systemd inside it.                                          | [Compatible nodes](https://github.com/nestybox/sysbox#host-requirements). |

On Kubernetes, gVisor, Kata, and Sysbox each install a [RuntimeClass](https://kubernetes.io/docs/concepts/containers/runtime-class/) on the nodes.
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

The RuntimeClass name is whatever the cluster administrator created, such as `gvisor`, `kata-qemu`, or `sysbox-runc`.
To let users pick a runtime, set `runtime_class_name` from a [parameter](./parameters/index.md).
To run sandboxed Pods on their own nodes, add a node selector and a [toleration](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/) for a tainted node pool.

[Workspace process logging](./process-logging.md) uses eBPF on the node, so it can't see the processes inside a gVisor or Kata sandbox.

Examples and references:

- [GKE Sandbox](https://cloud.google.com/kubernetes-engine/docs/concepts/sandbox-pods): gVisor node pools on GKE.
- [Pod Sandboxing on Azure Kubernetes Service (AKS)](https://learn.microsoft.com/en-us/azure/aks/use-pod-sandboxing): Kata node pools on AKS.
- [Kata Containers installation](https://github.com/kata-containers/kata-containers/blob/main/docs/install/README.md): install options for other clusters, including [Firecracker](https://firecracker-microvm.github.io/) as the hypervisor.
- [gVisor security model](https://gvisor.dev/docs/architecture_guide/security/): what gVisor protects against, and what it doesn't.
- [Envbox](./docker-in-workspaces.md#envbox) and the [kubernetes-envbox](https://registry.coder.com/templates/coder/kubernetes-envbox) template: run Sysbox inside a Pod when you can't install it on the nodes. The outer container is privileged, so Envbox is weaker than Sysbox on the nodes.
- [Sysbox security](https://github.com/nestybox/sysbox/blob/master/docs/user-guide/security.md): what the Sysbox user namespaces isolate.
- [Pod Security Standards](https://kubernetes.io/docs/concepts/security/pod-security-standards/): block privileged Pods in the namespace that runs agent workspaces.

## Network isolation

Network isolation works as a firewall for the agent.
It limits where the agent can send data, so a prompt injection can't send your code or credentials to an attacker's host.

The workspace always needs to reach the Coder server.
The Coder workspace agent relays connections through it, so the workspace needs no inbound access.
For the connections Coder uses, refer to [Outbound network access](../../admin/security/security-best-practices.md#outbound-network-access).

| Method                                                                                                | Provided by               | Rules by                                       | Description                                                                                                       |
|-------------------------------------------------------------------------------------------------------|---------------------------|------------------------------------------------|-------------------------------------------------------------------------------------------------------------------|
| [Kubernetes NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/) | Kubernetes network plugin | IP address and port                            | Built into Kubernetes. It can't name hosts, so it fits workloads that need a few fixed addresses.                 |
| [Agent Firewall](../../ai-governance/agent-firewall/index.md)                                         | Coder Premium             | Domain and HTTP method                         | Runs the AI agent's process in a firewall inside the workspace, and streams audit logs to Coder.                  |
| Egress proxy                                                                                          | Squid or another proxy    | Host name                                      | A proxy Pod allows only listed host names, and a NetworkPolicy lets the workspace reach only Coder and the proxy. |
| [NVIDIA OpenShell](https://docs.nvidia.com/openshell/latest/about/overview)                           | NVIDIA, open source       | Host name and the program that connects        | A proxy in the sandbox checks each connection against a policy, and Landlock limits file access.                  |
| [Cilium](https://docs.cilium.io/en/stable/security/dns/)                                              | Cilium network plugin     | Host name, IP address, port, and HTTP requests | Cilium's DNS proxy learns the addresses of allowed names, and its policies apply to the whole cluster.            |

> [!WARNING]
> Kubernetes accepts a NetworkPolicy even when the network plugin doesn't enforce it.
> Test each rule from inside a workspace before you run an agent in it.

Rules by host name still allow every path on an allowed host.
If an allowed host stores user content, such as a code host or a file share, an agent can still send data to it.

Agent Firewall wraps one process, so other processes in the workspace keep full network access.
A NetworkPolicy, a proxy, or Cilium applies to the whole Pod, so a command the agent starts in another way can't bypass it.
For why a boundary below the process is harder to bypass, refer to [Why network boundaries are more effective than process-level controls](../../coder-agents/platform-controls/template-optimization.md#why-network-boundaries-are-more-effective-than-process-level-controls).
You can use both: a Pod-level rule for all traffic, and Agent Firewall for an audit log of each request the agent makes.

Examples and references:

- [Network isolation for Coder Agents](../../coder-agents/platform-controls/agent-isolation.md): a template that runs the agent in its own Pod with a NetworkPolicy.
- [Agent Firewall example policy](../../ai-governance/agent-firewall/example-policy.md) and [rules engine](../../ai-governance/agent-firewall/rules-engine.md): allow rules by domain, path, and HTTP method.
- [Agent Firewall on Kubernetes](../../ai-governance/agent-firewall/nsjail/k8s.md) and [landjail](../../ai-governance/agent-firewall/landjail.md): the jail types and the container settings they need.
- [agent-firewall module](https://registry.coder.com/modules/coder/agent-firewall): install Agent Firewall and wrap an agent CLI in a template.
- [Squid access controls](https://www.squid-cache.org/Doc/config/acl/): allow lists by host name for an egress proxy.
- [Cilium policy language](https://docs.cilium.io/en/stable/security/policy/language/): rules by DNS name, port, and HTTP request.
- [OpenShell source](https://github.com/NVIDIA/OpenShell): the policy format and the sandbox image.

## Secret isolation

An agent can use any credential it can read, and a prompt injection can tell it to send that credential somewhere.
Keep credentials that the task doesn't need out of the workspace, and give the rest the smallest scope that works.

Every process that the Coder workspace agent starts, such as a terminal, an app, a startup script, or an AI agent, gets the workspace agent token in `CODER_AGENT_TOKEN`.
An AI agent that reads this token can use it to get:

- The workspace owner's Git SSH key.
- The owner's tokens for each [external auth](../../admin/external-auth.md) provider that the owner has linked.
- The [user secrets](../../workspaces/user-secrets.md) and [workspace secrets](../../admin/security/secrets.md#workspace-secrets) that Coder injects into the workspace.

To limit what an agent can read:

| Method                                                                | Provided by         | Description                                                                                                                         |
|-----------------------------------------------------------------------|---------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| [API key scopes](../../admin/users/sessions-tokens.md#api-key-scopes) | Coder               | Give the agent a token with only the scopes it needs, instead of the owner's session token.                                         |
| [AI Gateway](../../ai-governance/ai-gateway/index.md)                 | Coder Premium       | The agent calls model providers through Coder with a Coder token, so the provider API keys stay on the Coder server.                |
| [Service accounts](../../admin/users/headless-auth.md)                | Coder Premium       | Run autonomous agents as a headless user with no linked external auth, no personal secrets, and only the template access they need. |
| [Dynamic secrets](../../admin/security/secrets.md#dynamic-secrets)    | Your cloud provider | Create a short-lived credential or service account for each workspace in the template, tied to the workspace lifecycle.             |

`data.coder_workspace_owner.me.session_token` has the owner's full access to the Coder API.
The [AI Gateway client examples](../../ai-governance/ai-gateway/clients/index.md) inject it so the agent can authenticate.
If you use this pattern, run autonomous agents as a service account, so the token in the workspace has only that account's access.

Examples and references:

- [Scope permissions and credentials](../../coder-agents/platform-controls/template-optimization.md#scope-permissions-and-credentials) and [Git identity](../../coder-agents/platform-controls/template-optimization.md#git-identity): credentials and commit identity for agent templates.
- [API key scopes reference](../../reference/api-key-scopes.md): each scope and the permissions it grants.
- [External auth in templates](./external-auth.md): which providers a template requests, and when the owner links them.
- [AI Gateway authentication](../../ai-governance/ai-gateway/auth.md): how clients, gateway keys, and provider keys differ.
- [Amazon Elastic Kubernetes Service (EKS) Pod Identity](https://docs.aws.amazon.com/eks/latest/userguide/pod-identities.html), [GKE Workload Identity Federation](https://cloud.google.com/kubernetes-engine/docs/concepts/workload-identity), and [AKS Workload Identity](https://learn.microsoft.com/en-us/azure/aks/workload-identity-overview): give a workspace Pod a cloud identity without a static key.
- [Vault AWS secrets engine](https://developer.hashicorp.com/vault/docs/secrets/aws): create a short-lived AWS credential for each workspace.

## Audit agent activity

A sandbox limits what an agent can do. Logs show what it did.

| Method                                                                   | Provided by   | What it records                                                         |
|--------------------------------------------------------------------------|---------------|-------------------------------------------------------------------------|
| [Agent Firewall audit logs](../../ai-governance/agent-firewall/index.md) | Coder Premium | Each request the firewall allows or denies.                             |
| [AI Gateway audit](../../ai-governance/ai-gateway/audit.md)              | Coder Premium | Prompts, tool calls, and token use for each model request.              |
| [Workspace process logging](./process-logging.md)                        | Coder Premium | Each process started in the workspace container, with eBPF on the node. |
| [Connection logs](../../admin/monitoring/connection-logs.md)             | Coder Premium | SSH, app, and port-forward sessions into the workspace.                 |
| [Audit logs](../../admin/security/audit-logs.md)                         | Coder Premium | Changes to templates, workspaces, users, and tokens.                    |

## Registry templates

These templates in the [Coder Registry](https://registry.coder.com/templates) build agent sandboxes that combine several properties.
Each one is a starting point that you adapt to your cluster.

| Template                                                                                               | Fast startup                          | Kernel isolation        | Network isolation                         |
|--------------------------------------------------------------------------------------------------------|---------------------------------------|-------------------------|-------------------------------------------|
| [agent-sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-agent-sandbox) | Prebuilds or agent-sandbox warm pools | gVisor or Kata          | NetworkPolicy, with an optional guardrail |
| [Plain Sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-sandboxed)     | Prebuilds                             | Optional gVisor or Kata | Squid egress proxy by host name           |
| [OpenShell (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-openshell)         | Prebuilds                             | Optional Kata           | OpenShell rules by host name and program  |

## Learn more

- [Docker in workspaces](./docker-in-workspaces.md)
- [Template optimization for Coder Agents](../../coder-agents/platform-controls/template-optimization.md)
- [Agent Firewall](../../ai-governance/agent-firewall/index.md)
- [AI Gateway](../../ai-governance/ai-gateway/index.md)
- [Agent Relay](../../agent-relay/index.md)
- [Security best practices](../../admin/security/security-best-practices.md)
- [kubernetes-sigs/agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox)
