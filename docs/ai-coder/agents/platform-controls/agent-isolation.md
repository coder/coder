# Network isolation

Use your template to run the AI agent in a separate sandbox from the user's development environment.
You control which files and networks the agent can access without restricting the user's access.

This guide uses Kubernetes and shows two methods: a Kubernetes NetworkPolicy around a separate Pod for the agent, or [Agent Firewall](../../agent-firewall/index.md) around the agent's processes inside one Pod.
You can also use another sandbox, such as Bubblewrap with firewall rules.
Coder selects the workspace agent that runs chat tools.
You configure the sandbox around that agent.

> [!NOTE]
> Sandboxing in Coder is at an early stage and is subject to change.
> [Contact us](https://coder.com/contact) to share feedback.

## How it works

- The user's Pod keeps its normal development access.
- The AI agent's Pod runs as a non-root user with a network policy that allows Coder.
- Both Pods share project files, but have separate home directories.

Network policies select Pods, not individual containers.
Use separate Pods to give the user and AI agent different network access.
The Pods run on the same node and share one `ReadWriteOnce` persistent volume claim (PVC).
They do not need `ReadWriteMany` storage.

## Choose a method

| | NetworkPolicy, two Pods | Agent Firewall, one Pod |
|---|---|---|
| What is restricted | Every process in the agent's Pod, including workspace MCP servers | Shells the agent spawns: terminals, `coder ssh`, workspace app commands, and Coder Agents tool calls. The agent process and workspace MCP servers stay outside. |
| Rules | IP addresses and ports | Hostnames, HTTP methods, and paths |
| Audit | None built in | Every allowed and denied request, in the workspace and in the Coder server log |
| Cluster requirements | NetworkPolicy enforcement, same-node scheduling for the shared volume | `NET_ADMIN` and `SYS_ADMIN` capabilities on the agent's container, `sudo` in the image |
| Container hardening | Capabilities dropped, no privilege escalation, read-only root | Capabilities added, `sudo` limited to one command after startup |
| License | Any | Premium ([AI Governance](../../ai-governance.md)) |

Steps 1 to 3 set up the NetworkPolicy method.
[Alternative: Agent Firewall in one Pod](#alternative-agent-firewall-in-one-pod) covers the second method.
Use the NetworkPolicy method when you need every process in the agent's Pod restricted.
Use Agent Firewall when you need hostname rules or an audit log, or when the cluster does not enforce NetworkPolicy.

## 1. Configure the template

Use a test namespace and a workspace without sensitive data.
You need:

- A Linux cluster with NetworkPolicy enforcement turned on for standalone Pods.
- A default storage class that provides a volume writable by user and group ID `1000`.
- A fixed IPv4 address and TCP port for Coder, including agent downloads and relay connections.
- A Coder provisioner with permission to create Pods, a PVC, a ConfigMap, and a NetworkPolicy in that namespace.

Check these permissions using the provisioner's Kubernetes credentials:

```sh
kubectl auth can-i create pods -n YOUR_NAMESPACE
kubectl auth can-i create persistentvolumeclaims -n YOUR_NAMESPACE
kubectl auth can-i create configmaps -n YOUR_NAMESPACE
kubectl auth can-i create networkpolicies.networking.k8s.io -n YOUR_NAMESPACE
```

Each command must return `yes`.
The provisioner also needs permission to read, update, and delete these resources.

> [!WARNING]
> Do not run untrusted code until the network tests pass.
> Kubernetes can accept a NetworkPolicy without enforcing it, leaving the agent's network access unrestricted.

For GKE, check [network policy enforcement](https://cloud.google.com/kubernetes-engine/docs/how-to/network-policy).
For Amazon VPC CNI, use Deployment-managed Pods instead of this example's standalone Pods.
[AWS documents unreliable enforcement for standalone Pods](https://docs.aws.amazon.com/eks/latest/userguide/cni-network-policy.html).

Save this Terraform as `main.tf` in an empty template directory.
Edit the `locals` block at the top of the file with your namespace and Coder server address.

<details>
<summary>main.tf: two Pods, one project volume, and an agent network policy</summary>

```tf
terraform {
  required_providers {
    coder      = { source = "coder/coder" }
    kubernetes = { source = "hashicorp/kubernetes" }
  }
}

locals {
  # Edit these values for your cluster and Coder deployment.

  # Use ~/.kube/config on the provisioner host instead of in-cluster authentication.
  use_kubeconfig = false
  # Existing namespace for workspace resources.
  namespace = "coder-sandbox-test"
  # Storage class for the shared project PVC. Null uses the cluster default.
  storage_class_name = null
  # Hostname in the Coder access URL, without scheme or port.
  coder_host = "coder.example.com"
  # Trusted fixed IPv4 address of the external Coder endpoint, not a cluster Service or node IP.
  coder_ip = "192.0.2.10"
  # TCP port of the Coder endpoint (including agent downloads and embedded DERP).
  coder_port = 443
}

provider "kubernetes" {
  config_path = local.use_kubeconfig ? "~/.kube/config" : null
}

data "coder_workspace" "me" {}
data "coder_provisioner" "me" {}

resource "coder_agent" "dev" {
  os   = "linux"
  arch = data.coder_provisioner.me.arch
  dir  = "/home/coder/project"
}
resource "coder_agent" "dev-coderd-chat" {
  os   = "linux"
  arch = data.coder_provisioner.me.arch
  dir  = "/home/coder/project"
  env  = { CODER_AGENT_EXP_MCP_CONFIG_FILES = "/etc/coder/mcp.json" }
}

locals {
  name   = "coder-${data.coder_workspace.me.id}"
  labels = { "com.coder.workspace.id" = data.coder_workspace.me.id }
}

resource "kubernetes_persistent_volume_claim_v1" "project" {
  metadata {
    name      = "${local.name}-project"
    namespace = local.namespace
  }
  wait_until_bound = false
  spec {
    access_modes       = ["ReadWriteOnce"]
    storage_class_name = local.storage_class_name
    resources {
      requests = { storage = "10Gi" }
    }
  }
}

resource "kubernetes_config_map_v1" "mcp" {
  metadata {
    name      = "${local.name}-mcp"
    namespace = local.namespace
  }
  data = { "mcp.json" = jsonencode({ mcpServers = {} }) }
}

resource "kubernetes_network_policy_v1" "sandbox" {
  metadata {
    name      = "${local.name}-sandbox"
    namespace = local.namespace
  }
  spec {
    pod_selector {
      match_labels = merge(local.labels, { "coder.com/agent-role" = "sandbox" })
    }
    policy_types = ["Egress"]
    egress {
      to {
        ip_block {
          cidr = "${local.coder_ip}/32"
        }
      }
      ports {
        protocol = "TCP"
        port     = local.coder_port
      }
    }
  }
}

resource "kubernetes_pod_v1" "dev" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = "${local.name}-dev"
    namespace = local.namespace
    labels    = merge(local.labels, { "coder.com/agent-role" = "dev" })
  }
  spec {
    automount_service_account_token = false
    host_network                    = false
    security_context {
      run_as_user     = 1000
      run_as_group    = 1000
      fs_group        = 1000
      run_as_non_root = true
      seccomp_profile { type = "RuntimeDefault" }
    }
    host_aliases {
      ip        = local.coder_ip
      hostnames = [local.coder_host]
    }
    container {
      name        = "dev"
      image       = "codercom/example-base:ubuntu"
      command     = ["sh", "-c", coder_agent.dev.init_script]
      working_dir = "/home/coder/project"
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.dev.token
      }
      volume_mount {
        name       = "home"
        mount_path = "/home/coder"
      }
      volume_mount {
        name       = "project"
        mount_path = "/home/coder/project"
      }
      volume_mount {
        name       = "tmp"
        mount_path = "/tmp"
      }
    }
    volume {
      name = "project"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim_v1.project.metadata[0].name
      }
    }
    volume {
      name = "home"
      empty_dir {}
    }
    volume {
      name = "tmp"
      empty_dir {}
    }
  }
}

resource "kubernetes_pod_v1" "sandbox" {
  count      = data.coder_workspace.me.start_count
  depends_on = [kubernetes_network_policy_v1.sandbox]
  metadata {
    name      = "${local.name}-dev-coderd-chat"
    namespace = local.namespace
    labels    = merge(local.labels, { "coder.com/agent-role" = "sandbox" })
  }
  spec {
    automount_service_account_token = false
    host_network                    = false
    security_context {
      run_as_user     = 1000
      run_as_group    = 1000
      fs_group        = 1000
      run_as_non_root = true
      seccomp_profile {
        type = "RuntimeDefault"
      }
    }
    # Dev schedules independently, including with WaitForFirstConsumer storage.
    affinity {
      pod_affinity {
        required_during_scheduling_ignored_during_execution {
          topology_key = "kubernetes.io/hostname"
          label_selector {
            match_labels = merge(local.labels, { "coder.com/agent-role" = "dev" })
          }
        }
      }
    }
    host_aliases {
      ip        = local.coder_ip
      hostnames = [local.coder_host]
    }
    container {
      name        = "dev-coderd-chat"
      image       = "codercom/example-base:ubuntu"
      command     = ["sh", "-c", coder_agent.dev-coderd-chat.init_script]
      working_dir = "/home/coder/project"
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.dev-coderd-chat.token
      }
      env {
        name  = "CODER_AGENT_EXP_MCP_CONFIG_FILES"
        value = "/etc/coder/mcp.json"
      }
      security_context {
        allow_privilege_escalation = false
        read_only_root_filesystem  = true
        run_as_non_root            = true
        capabilities { drop = ["ALL"] }
      }
      volume_mount {
        name       = "home"
        mount_path = "/home/coder"
      }
      volume_mount {
        name       = "project"
        mount_path = "/home/coder/project"
      }
      volume_mount {
        name       = "tmp"
        mount_path = "/tmp"
      }
      volume_mount {
        name       = "mcp"
        mount_path = "/etc/coder"
        read_only  = true
      }
    }
    volume {
      name = "project"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim_v1.project.metadata[0].name
      }
    }
    volume {
      name = "home"
      empty_dir {}
    }
    volume {
      name = "tmp"
      empty_dir {}
    }
    volume {
      name = "mcp"
      config_map {
        name = kubernetes_config_map_v1.mcp.metadata[0].name
      }
    }
  }
}
```

</details>

Use the Coder hostname without `https://` or a port.
Use an external IP address, not a cluster Service or node IP.
`hostAliases` lets the agent find Coder at this IP without DNS.
If your cluster has no default storage class, set `storage_class_name` to one that fits the requirements above.

If the provisioner runs outside the cluster, set `use_kubeconfig = true`.
Provide `~/.kube/config` on the provisioner host.
Otherwise, the example uses in-cluster authentication.

The AI agent runs as user ID `1000`, without Linux capabilities or a Kubernetes service-account token.
It can write to its home, temporary directories, and shared project, but not system files.
It uses the runtime's default security profile.

The network policy allows Coder's IP and TCP port, but not DNS.
Coder selects the agent whose name ends in `-coderd-chat`.
Use this ending on exactly one top-level agent.

## 2. Create a workspace

Sign in to Coder.
From the template directory, run:

```sh
terraform init
terraform validate
coder templates push sandbox-k8s --directory .
coder create sandbox-test --template sandbox-k8s
```

After the template passes validation, wait for both agents to connect.
In **Agents**, start a chat attached to `sandbox-test`.
Ask the chat to run:

```sh
id -u
touch /home/coder/project/sandbox-check
touch /etc/sandbox-check
```

The first command must print `1000`.
The agent must create the project file but fail to write to `/etc`.
Read each command's output, not only the chat summary.

## 3. Test network access

Try GitHub and the npm registry from the user's terminal:

```sh
curl --noproxy '*' --head --max-time 10 https://github.com
curl --noproxy '*' --head --max-time 10 https://registry.npmjs.org
```

Both requests must return HTTP headers.
Then ask the chat to run the same commands.
Both must fail with a DNS error, a connection error, or a timeout.
The chat must still be able to run commands through Coder.

A DNS error does not prove that the policy blocks connections to IP addresses.
Find GitHub's IP address from the user's Pod.
Repeat the request from the user's terminal and the chat with that address:

```sh
# Run in the user's Pod.
getent ahostsv4 github.com

# Replace GITHUB_IP with an address from that output. Run in both Pods.
curl --noproxy '*' --head --max-time 10 --resolve github.com:443:GITHUB_IP https://github.com
```

The user's request must succeed, and the agent's request must fail to connect.
If the agent reaches either site, stop the test workspace.
Fix policy enforcement before continuing.

To test PostgreSQL, use a test database you control.
Run `SELECT 1` from the user's terminal.
Ask the chat to run the same query.
The user's request must succeed.
The agent's request must fail to connect.
A login error does not count as a blocked connection.

If the user cannot reach GitHub or npm, fix that connection before testing the agent.

## Alternative: Agent Firewall in one Pod

Agent Firewall runs the agent's shells in a network namespace whose only exit is a proxy that enforces an allowlist.
This method keeps both agents in one Pod with two containers, so the Pod needs no NetworkPolicy and no node affinity.
The user's container is unchanged.
The AI agent's container starts the agent, then starts one Agent Firewall jail and routes every shell into it.

> [!NOTE]
> Agent Firewall is part of [AI Governance](../../ai-governance.md), which is included with a Premium license.

The Coder agent itself runs outside the jail.
Agent Firewall's proxy does not carry WebSocket connections, and the agent uses WebSockets to reach Coder.
Two wrappers put the agent's children inside:

- The workspace user's login shell, which covers terminals, `coder ssh`, and workspace app commands.
- A `sh` first on the agent's `PATH`, which covers Coder Agents tool calls and the agent's process API.
  Both spawn `sh -c`, not the login shell.

Template startup scripts run outside the jail, so a script can start a service that Coder must reach.
Do not put untrusted commands in startup scripts.

You need:

- A workspace image with `sudo` without a password, `setpriv`, `nsenter`, `iptables`, `curl`, and `python3`.
  `codercom/example-base:ubuntu` has all of them.
- Nodes that allow the `NET_ADMIN` and `SYS_ADMIN` capabilities.
  Refer to [nsjail on Kubernetes](../../agent-firewall/nsjail/k8s.md) for node-specific requirements.
- A Coder provisioner with permission to create Pods, a PVC, and a ConfigMap in the namespace.

Save these two files in an empty template directory.
Edit the `locals` block in `main.tf` with your namespace, Coder hostname, and the hosts the agent may reach.

<details>
<summary>main.tf: one Pod, two containers, Agent Firewall around the agent's shells</summary>

```tf
terraform {
  required_providers {
    coder      = { source = "coder/coder" }
    kubernetes = { source = "hashicorp/kubernetes" }
  }
}

locals {
  # Edit these values for your cluster and Coder deployment.

  # Use ~/.kube/config on the provisioner host instead of in-cluster authentication.
  use_kubeconfig = false
  # Existing namespace for workspace resources.
  namespace = "coder-sandbox-test"
  # Storage class for the shared project PVC. Null uses the cluster default.
  storage_class_name = null
  # Hostname in the Coder access URL, without scheme or port.
  coder_host = "coder.example.com"
  # Other hosts the AI agent may reach, for example an internal artifact server.
  allowed_hosts = ["artifacts.example.com"]
}

provider "kubernetes" {
  config_path = local.use_kubeconfig ? "~/.kube/config" : null
}

data "coder_workspace" "me" {}
data "coder_provisioner" "me" {}

resource "coder_agent" "dev" {
  os   = "linux"
  arch = data.coder_provisioner.me.arch
  dir  = "/home/coder/project"
}
resource "coder_agent" "dev-coderd-chat" {
  os   = "linux"
  arch = data.coder_provisioner.me.arch
  dir  = "/home/coder/project"
  env  = { CODER_AGENT_EXP_MCP_CONFIG_FILES = "/etc/coder/mcp.json" }
}

locals {
  name   = "coder-${data.coder_workspace.me.id}"
  labels = { "com.coder.workspace.id" = data.coder_workspace.me.id }
}

resource "kubernetes_persistent_volume_claim_v1" "project" {
  metadata {
    name      = "${local.name}-project"
    namespace = local.namespace
  }
  wait_until_bound = false
  spec {
    access_modes       = ["ReadWriteOnce"]
    storage_class_name = local.storage_class_name
    resources {
      requests = { storage = "10Gi" }
    }
  }
}

resource "kubernetes_config_map_v1" "mcp" {
  metadata {
    name      = "${local.name}-mcp"
    namespace = local.namespace
  }
  data = { "mcp.json" = jsonencode({ mcpServers = {} }) }
}

resource "kubernetes_pod_v1" "workspace" {
  count = data.coder_workspace.me.start_count
  metadata {
    name      = local.name
    namespace = local.namespace
    labels    = local.labels
  }
  spec {
    automount_service_account_token = false
    host_network                    = false
    security_context {
      run_as_user     = 1000
      run_as_group    = 1000
      fs_group        = 1000
      run_as_non_root = true
      seccomp_profile { type = "RuntimeDefault" }
    }

    # The user's container. Normal network access, no extra capabilities.
    container {
      name        = "dev"
      image       = "codercom/example-base:ubuntu"
      command     = ["sh", "-c", coder_agent.dev.init_script]
      working_dir = "/home/coder/project"
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.dev.token
      }
      security_context {
        allow_privilege_escalation = false
        capabilities { drop = ["ALL"] }
      }
      volume_mount {
        name       = "dev-home"
        mount_path = "/home/coder"
      }
      volume_mount {
        name       = "project"
        mount_path = "/home/coder/project"
      }
    }

    # The AI agent's container. Every shell it spawns runs inside Agent Firewall.
    container {
      name        = "dev-coderd-chat"
      image       = "codercom/example-base:ubuntu"
      command     = ["sh", "-c", file("${path.module}/agent-firewall.sh")]
      working_dir = "/home/coder/project"
      env {
        name  = "CODER_AGENT_TOKEN"
        value = coder_agent.dev-coderd-chat.token
      }
      env {
        name  = "CODER_AGENT_INIT"
        value = coder_agent.dev-coderd-chat.init_script
      }
      env {
        name  = "BOUNDARY_ALLOW_HOSTS"
        value = join(" ", concat([local.coder_host], local.allowed_hosts))
      }
      security_context {
        capabilities { add = ["NET_ADMIN", "SYS_ADMIN"] }
      }
      volume_mount {
        name       = "chat-home"
        mount_path = "/home/coder"
      }
      volume_mount {
        name       = "project"
        mount_path = "/home/coder/project"
      }
      volume_mount {
        name       = "mcp"
        mount_path = "/etc/coder"
        read_only  = true
      }
    }

    volume {
      name = "project"
      persistent_volume_claim {
        claim_name = kubernetes_persistent_volume_claim_v1.project.metadata[0].name
      }
    }
    volume {
      name = "dev-home"
      empty_dir {}
    }
    volume {
      name = "chat-home"
      empty_dir {}
    }
    volume {
      name = "mcp"
      config_map {
        name = kubernetes_config_map_v1.mcp.metadata[0].name
      }
    }
  }
}
```

</details>

<details>
<summary>agent-firewall.sh: the AI agent's container entrypoint</summary>

```sh
#!/bin/sh
# Container entrypoint for the AI agent: run the Coder agent, and run every
# shell it spawns inside Coder Agent Firewall (boundary).
#
# Inputs (environment):
#   CODER_AGENT_INIT      the coder_agent init script
#   BOUNDARY_ALLOW_HOSTS  space-separated hosts to allow, Coder's host first
#   BOUNDARY_VERSION      boundary release to install (default v0.11.0)
#
# The agent itself runs outside the jail: boundary's proxy does not pass
# WebSocket upgrades, and the agent needs them to reach Coder. Two wrappers
# put its children inside: the login shell (terminals, coder ssh, apps) and
# `sh` on the agent's PATH (Coder Agents tool calls, the process API).
set -eu

BIN="$HOME/.local/bin"
CONF="$HOME/.config/coder_boundary"
mkdir -p "$BIN/jail-path" "$CONF" /tmp/boundary_logs

# 1. Install boundary and write its config.
if [ ! -x "$BIN/boundary" ]; then
  curl -fsSL "https://github.com/coder/boundary/releases/download/${BOUNDARY_VERSION:-v0.11.0}/boundary-linux-amd64.tar.gz" | tar -xz -C "$BIN"
  mv "$BIN/boundary-linux-amd64" "$BIN/boundary"
  chmod 755 "$BIN/boundary"
fi
{
  echo "allowlist:"
  for host in $BOUNDARY_ALLOW_HOSTS; do echo "  - \"domain=$host\""; done
  cat <<'EOF'
jail_type: nsjail
# The jail's namespace must belong to the container's user namespace so
# that root can nsenter it for each shell.
no_user_namespace: true
log_dir: /tmp/boundary_logs
log_level: info
proxy_port: 8087
EOF
} > "$CONF/config.yaml"

# 2. Wrappers that join the jail. Both pass through untouched for template
# startup scripts (CODER_SCRIPT_DATA_DIR) and for shells already inside.
for target in /bin/bash:jailed-shell /bin/sh:jail-path/sh; do
  real=${target%%:*}
  file=${target#*:}
  cat > "$BIN/$file" <<EOF
#!/bin/bash
if [ -n "\${CODER_SCRIPT_DATA_DIR:-}" ] || [ -n "\${BOUNDARY_JAIL:-}" ]; then
  exec $real "\$@"
fi
for i in \$(seq 1 100); do [ -s /tmp/boundary-jail.env ] && break; sleep 0.1; done
exec sudo -E nsenter --net=/tmp/boundary-netns \\
  setpriv --reuid 1000 --regid 1000 --init-groups \\
  env BOUNDARY_JAIL=1 "PATH=\$PATH" \$(cat /tmp/boundary-jail.env | tr '\\n' ' ') $real "\$@"
EOF
  chmod 755 "$BIN/$file"
done
sudo usermod -s "$BIN/jailed-shell" "$(id -un)"
export PATH="$BIN/jail-path:$PATH"

# 3. The agent, outside the jail. Start it first so boundary can attach to
# the agent's audit socket and stream decisions to Coder.
/bin/sh -c "$CODER_AGENT_INIT" &
AGENT_PID=$!
for i in $(seq 1 300); do [ -S /tmp/boundary-audit.sock ] && break; sleep 0.1; done

# 4. One jail for the whole container. The process inside publishes a handle
# to its network namespace, because root cannot read /proc/<pid>/ns of a
# process that holds capabilities. mount(8) refuses to run as non-root, so
# the syscall is made from Python.
rm -f /tmp/boundary-jail.env /tmp/boundary-netns
touch /tmp/boundary-netns
"$BIN/boundary" -- /bin/sh -c '
  env | grep -E "^(SSL_CERT_FILE|SSL_CERT_DIR|CURL_CA_BUNDLE|GIT_SSL_CAINFO|REQUESTS_CA_BUNDLE|NODE_EXTRA_CA_CERTS)=" > /tmp/boundary-jail.env.tmp
  python3 -c "import ctypes; l=ctypes.CDLL(None, use_errno=True); assert l.mount(b\"/proc/self/ns/net\", b\"/tmp/boundary-netns\", None, 4096, None) == 0"
  mv /tmp/boundary-jail.env.tmp /tmp/boundary-jail.env
  exec sleep infinity' &
for i in $(seq 1 100); do [ -s /tmp/boundary-jail.env ] && break; sleep 0.1; done
[ -s /tmp/boundary-jail.env ] || { echo "boundary jail did not start" >&2; exit 1; }

# 5. Now that boundary has escalated once, replace the image's passwordless
# sudo with one rule: the exact command the wrappers use to enter the jail.
# Otherwise anything in the jail could run `sudo iptables` or `sudo nsenter`
# and walk out.
user=$(id -un)
sudo /bin/sh -c "
  printf '%s\n' '$user ALL=(root) NOPASSWD:SETENV: /usr/bin/nsenter --net=/tmp/boundary-netns setpriv --reuid 1000 --regid 1000 --init-groups env *' > /etc/sudoers.d/00-boundary
  chmod 440 /etc/sudoers.d/00-boundary
  find /etc/sudoers.d -type f ! -name 00-boundary ! -name README -delete
  sed -i '/NOPASSWD/d' /etc/sudoers
  visudo -c -q
"

wait "$AGENT_PID"
```

</details>

The script installs Agent Firewall from its GitHub release on first start, so the Pod needs internet access at that point.
To avoid the download, install the binary in your image and remove that step.
The allowlist uses `domain=` rules, which include subdomains.
Refer to the [rules engine documentation](../../agent-firewall/rules-engine.md) for methods and paths.

After the jail is up, the script replaces the image's passwordless `sudo` with one rule that allows the exact command the wrappers use to enter the jail.
Without that step, a process inside the jail could run `sudo iptables` or `sudo nsenter` and leave it.

Push the template and create a workspace as in [step 2](#2-create-a-workspace).
Then test from the user's terminal and from the chat, as in [step 3](#3-test-network-access), with one difference: a blocked request returns HTTP `403` from Agent Firewall instead of a connection error.
Ask the chat to run:

```sh
curl -sS --head --max-time 10 https://github.com
curl -sS --head --max-time 10 https://artifacts.example.com
sudo -n true
```

GitHub must return `HTTP/1.1 403 Forbidden` with the body `Request Blocked by Boundary`.
The allowed host must return its normal response.
`sudo` must fail with `a password is required`.
The denied request also appears in the Coder server log as a `boundary_request` entry with `decision=deny`.

## Limits

Other NetworkPolicies can allow connections that this policy blocks.
Your network plugin can also allow traffic to the node or its local DNS service.
Review these exceptions in the [Kubernetes NetworkPolicy documentation](https://kubernetes.io/docs/concepts/services-networking/network-policies/).

The agent can read any credentials you give it, edit shared files, and send data to Coder.
Keep workspace MCP configuration outside the shared project.
This policy does not restrict organization MCP servers.
Restrict their tools and credentials separately.

If you use another sandbox, apply it to the full workspace agent and its child processes where the sandbox allows it.
Restricting only a shell leaves file tools and workspace MCP processes outside the sandbox.
The Agent Firewall method has this limit: the agent's file tools and workspace MCP servers run outside the jail, and Agent Firewall does not yet filter UDP traffic other than DNS.
It also needs `NET_ADMIN` and `SYS_ADMIN` on the agent's container, which the NetworkPolicy method does not.
