# Network isolation

Use your template to run the AI agent in a separate sandbox from the user's development environment.
You control which files and networks the agent can access without restricting the user's access.

This guide uses Kubernetes, but you can use another sandbox, such as Bubblewrap with firewall rules.
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

## Limits

Other NetworkPolicies can allow connections that this policy blocks.
Your network plugin can also allow traffic to the node or its local DNS service.
Review these exceptions in the [Kubernetes NetworkPolicy documentation](https://kubernetes.io/docs/concepts/services-networking/network-policies/).

The agent can read any credentials you give it, edit shared files, and send data to Coder.
Keep workspace MCP configuration outside the shared project.
This policy does not restrict organization MCP servers.
Restrict their tools and credentials separately.

If you use another sandbox, apply it to the full workspace agent and its child processes.
Restricting only a shell leaves file tools and workspace MCP processes outside the sandbox.
