---
title: Sandbox workspaces
---

A sandbox is a workspace that limits what code inside it can reach, so you can run an AI agent or other untrusted code with less risk.
This page is for template authors and platform engineers who build those workspaces.
It describes the four properties a sandbox can combine, and which ones Coder provides and which come from a runtime, network plugin, or proxy you add to the template.

| Property                                | What it limits                                      | Coder provides                              | You add with the template                              |
|-----------------------------------------|-----------------------------------------------------|---------------------------------------------|--------------------------------------------------------|
| [Fast startup](#fast-startup)           | Nothing, but a slow sandbox doesn't get used        | Prebuilt workspaces and the build timeline  | A cluster warm pool, such as agent-sandbox             |
| [Kernel isolation](#kernel-isolation)   | Access to the host kernel and other workloads on it | VM-based templates                          | A container runtime such as gVisor, Kata, or Sysbox    |
| [Network isolation](#network-isolation) | The hosts and ports the workspace can connect to    | Agent Firewall                              | A NetworkPolicy, an egress proxy, OpenShell, or Cilium |
| [Secret isolation](#secret-isolation)   | The credentials that code in the workspace can read | Scoped tokens, AI Gateway, service accounts | Short-lived cloud credentials for each workspace       |

Each property is independent, so you can pick the ones your workloads need.
A template applies them per workspace, and [parameters](./parameters/index.md) or [presets](./parameters/workspace-presets.md) let users or groups pick a profile, such as a stricter network for one team.

For a template that runs the AI agent in a separate Pod from the user, refer to [Network isolation](../../coder-agents/platform-controls/agent-isolation.md) in the Coder Agents docs.

## Fast startup

Agents often start a new workspace for each task.
If the sandbox takes minutes to start, users skip it.

| Method                                                                       | Provided by        | Description                                                                                        |
|------------------------------------------------------------------------------|--------------------|----------------------------------------------------------------------------------------------------|
| [Prebuilt workspaces](./prebuilt-workspaces.md)                              | Coder Premium      | Coder keeps a pool of ready workspaces for each preset, and a user claims one at create.           |
| [Speed up templates](../../workspaces/speed-up-templates.md)                 | Coder              | Find slow steps with the build timeline, and move tools into the image instead of startup scripts. |
| [agent-sandbox](https://github.com/kubernetes-sigs/agent-sandbox) warm pools | Kubernetes project | A `SandboxWarmPool` keeps booted sandboxes ready in the cluster. It doesn't need Coder Premium.    |

A claim changes the workspace owner, so a resource that uses the owner's name in an immutable field is replaced at claim, and the claim is slow.
To keep the claim fast, refer to [Prevent resource replacement](./prebuilt-workspaces.md#prevent-resource-replacement).

## Kernel isolation

A standard container shares the node's kernel with every other container on the node.
A kernel bug or container escape in one workspace can reach the node and the other workloads on it.
Kernel isolation gives each workspace its own kernel, or a kernel that runs in user space.

| Method                                                               | Provided by          | Description                                                                                                                        | Requirements                                                              |
|----------------------------------------------------------------------|----------------------|------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------------------------------|
| VM-based templates                                                   | Coder and your cloud | Each workspace is its own virtual machine, such as the [AWS Linux](https://registry.coder.com/templates/coder/aws-linux) template. | Slower to start than a container.                                         |
| [gVisor](https://gvisor.dev/docs/user_guide/containerd/quick_start/) | Google, open source  | A kernel that runs in user space and handles the workspace's system calls.                                                         | Any Linux node. Slows file-heavy work, such as a large `git clone`.       |
| [Kata Containers](https://katacontainers.io/)                        | Open source          | Each Pod runs in a small VM with its own kernel, with a hypervisor such as QEMU or Cloud Hypervisor.                               | Bare metal nodes, or VMs with nested virtualization.                      |
| [Sysbox](./docker-in-workspaces.md#sysbox-container-runtime)         | Open source          | Isolates the workspace with Linux user namespaces, and runs Docker and systemd inside it.                                          | [Compatible nodes](https://github.com/nestybox/sysbox#host-requirements). |

On Kubernetes, gVisor, Kata, and Sysbox each install a RuntimeClass on the nodes.
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

## Network isolation

Network isolation works as a firewall for the workspace.
It limits where an AI agent can send data, so a prompt injection can't send your code or credentials to an attacker's host.

The workspace always needs to reach the Coder server.
The agent relays connections through it, so the workspace needs no inbound access.
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
> Test each rule from inside a workspace before you run untrusted code in it.

Rules by host name still allow every path on an allowed host.
If an allowed host stores user content, such as a code host or a file share, an agent can still send data to it.

## Secret isolation

An AI agent can use any credential it can read.
Keep credentials that the task doesn't need out of the workspace, and give the rest the smallest scope that works.

Every process that the Coder agent starts, such as a terminal, an app, or a startup script, gets the agent token in `CODER_AGENT_TOKEN`.
Code that holds the agent token can read these:

- The workspace owner's Git SSH key.
- The owner's tokens for each [external auth](../../admin/external-auth.md) provider that the owner has linked.
- The [user secrets](../../workspaces/user-secrets.md) and [workspace secrets](../../admin/security/secrets.md#workspace-secrets) that Coder injects into the workspace.

To limit what an agent can read:

| Method                                                                | Provided by         | Description                                                                                                                         |
|-----------------------------------------------------------------------|---------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| [API key scopes](../../admin/users/sessions-tokens.md#api-key-scopes) | Coder               | Give tools in the workspace a token with only the scopes they need, instead of the owner's session token.                           |
| [AI Gateway](../../ai-governance/ai-gateway/index.md)                 | Coder Premium       | The agent calls model providers through Coder with a Coder token, so the provider API keys stay on the Coder server.                |
| [Service accounts](../../admin/users/headless-auth.md)                | Coder Premium       | Run autonomous agents as a headless user with no linked external auth, no personal secrets, and only the template access they need. |
| [Dynamic secrets](../../admin/security/secrets.md#dynamic-secrets)    | Your cloud provider | Create a short-lived credential or service account for each workspace in the template, tied to the workspace lifecycle.             |

Do not pass `data.coder_workspace_owner.me.session_token` into the workspace.
It has the owner's full access to the Coder API.

## Registry templates

These templates in the [Coder Registry](https://registry.coder.com/templates) combine several properties.
Each one is a starting point that you adapt to your cluster.

| Template                                                                                               | Fast startup                          | Kernel isolation        | Network isolation                         |
|--------------------------------------------------------------------------------------------------------|---------------------------------------|-------------------------|-------------------------------------------|
| [agent-sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-agent-sandbox) | Prebuilds or agent-sandbox warm pools | gVisor or Kata          | NetworkPolicy, with an optional guardrail |
| [Plain Sandbox (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-sandboxed)     | Prebuilds                             | Optional gVisor or Kata | Squid egress proxy by host name           |
| [OpenShell (Kubernetes)](https://registry.coder.com/templates/coder-labs/kubernetes-openshell)         | Prebuilds                             | Optional Kata           | OpenShell rules by host name and program  |

## Learn more

- [Docker in workspaces](./docker-in-workspaces.md)
- [Network isolation for Coder Agents](../../coder-agents/platform-controls/agent-isolation.md)
- [Agent Firewall](../../ai-governance/agent-firewall/index.md)
- [Security best practices](../../admin/security/security-best-practices.md)
