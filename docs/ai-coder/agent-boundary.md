# Agent Boundary

Agent Boundaries are process-level firewalls that restrict and audit what autonomous programs, such as AI agents, can access and use.

![Screenshot of Agent Boundaries blocking a process](../images/guides/ai-agents/boundary.png)Example of Agent Boundaries blocking a process.

## Supported Agents

Agent Boundaries support the securing of any terminal-based agent, including your own custom agents.

## Features

Agent Boundaries offer network policy enforcement, which blocks domains and HTTP verbs to prevent exfiltration, and writes logs to the workspace.

## Getting Started with Boundary

The easiest way to use Agent Boundaries is through existing Coder modules, such as the [Claude Code module](https://registry.coder.com/modules/coder/claude-code). It can also be ran directly in the terminal by installing the [CLI](https://github.com/coder/boundary).

There are two supported ways to configure Boundary today:

1. **Inline module configuration** – fastest for quick testing.
2. **External `config.yaml`** – best when you need a large allow list or want everyone who launches Boundary manually to share the same config.

### Option 1: Inline module configuration (quick start)

Put every setting directly in the Terraform module when you just want to experiment:

```tf
module "claude-code" {
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = "4.1.0"
  enable_boundary     = true
  boundary_version    = "v0.2.0"
  boundary_log_dir    = "/tmp/boundary_logs"
  boundary_log_level  = "WARN"
  boundary_additional_allowed_urls = ["domain=google.com"]
  boundary_proxy_port = "8087"
}
```

All Boundary knobs live in Terraform, so you can iterate quickly without creating extra files.

### Option 2: Keep policy in `config.yaml` (extensive allow lists)

When you need to maintain a long allow list or share a detailed policy with teammates, keep Terraform minimal and move the rest into `config.yaml`:

```tf
module "claude-code" {
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = "4.1.0"
  enable_boundary     = true
  boundary_version    = "v0.2.0"
}
```

Then create a `config.yaml` file in your template directory with your policy:

```yaml
allowlist:
  - "domain=google.com"
  - "method=GET,HEAD domain=api.github.com"
  - "method=POST domain=api.example.com path=/users,/posts"
log_dir: /tmp/boundary_logs
proxy_port: 8087
log_level: warn
```

Add a `coder_script` resource to mount the configuration file into the workspace filesystem:

```tf
resource "coder_script" "boundary_config_setup" {
  agent_id     = coder_agent.dev.id
  display_name = "Boundary Setup Configuration"
  run_on_start = true

  script = <<-EOF
    #!/bin/sh
    mkdir -p ~/.config/coder_boundary
    echo '${base64encode(file("${path.module}/config.yaml"))}' | base64 -d > ~/.config/coder_boundary/config.yaml
    chmod 600 ~/.config/coder_boundary/config.yaml
  EOF
}
```

Boundary automatically reads `config.yaml` from `~/.config/coder_boundary/` when it starts, so everyone who launches Boundary manually inside the workspace picks up the same configuration without extra flags. This is especially convenient for managing extensive allow lists in version control.

- `boundary_version` defines what version of Boundary is being applied. This is set to `v0.2.0`, which points to the v0.2.0 release tag of `coder/boundary`.
- `boundary_log_dir` is the directory where log files are written to when the workspace spins up.
- `boundary_log_level` defines the verbosity at which requests are logged. Boundary uses the following verbosity levels:
  - `WARN`: logs only requests that have been blocked by Boundary
  - `INFO`: logs all requests at a high level
  - `DEBUG`: logs all requests in detail
- `boundary_additional_allowed_urls`: defines the URLs that the agent can access, in addition to the default URLs required for the agent to work. Rules use the format `"key=value [key=value ...]"`:
  - `domain=github.com` - allows the domain and all its subdomains
  - `domain=*.github.com` - allows only subdomains (the specific domain is excluded)
  - `method=GET,HEAD domain=api.github.com` - allows specific HTTP methods for a domain
  - `method=POST domain=api.example.com path=/users,/posts` - allows specific methods, domain, and paths
  - `path=/api/v1/*,/api/v2/*` - allows specific URL paths

You can also run Agent Boundaries directly in your workspace and configure it per template. You can do so by installing the [binary](https://github.com/coder/boundary) into the workspace image or at start-up. You can do so with the following command:

```hcl
curl -fsSL https://raw.githubusercontent.com/coder/boundary/main/install.sh | bash
 ```

## Runtime & Permission Requirements for Running the Boundary in Docker

This section describes the Linux capabilities and runtime configurations required to run the Agent Boundary inside a Docker container. Requirements vary depending on the OCI runtime and the seccomp profile in use.

### 1. Default `runc` runtime with `CAP_NET_ADMIN`

When using Docker’s default `runc` runtime, the Boundary requires the container to have `CAP_NET_ADMIN`. This is the minimal capability needed for configuring virtual networking inside the container.

Docker’s default seccomp profile may also block certain syscalls (such as `clone`) required for creating unprivileged network namespaces. If you encounter these restrictions, you may need to update or override the seccomp profile to allow these syscalls.

[see Docker Seccomp Profile Considerations](#docker-seccomp-profile-considerations)

### 2. Default `runc` runtime with `CAP_SYS_ADMIN` (testing only)

For development or testing environments, you may grant the container `CAP_SYS_ADMIN`, which implicitly bypasses many of the restrictions in Docker’s default seccomp profile.

- The Boundary does not require `CAP_SYS_ADMIN` itself.
- However, Docker’s default seccomp policy commonly blocks namespace-related syscalls unless `CAP_SYS_ADMIN` is present.
- Granting `CAP_SYS_ADMIN` enables the Boundary to run without modifying the seccomp profile.

⚠️ Warning: `CAP_SYS_ADMIN` is extremely powerful and should not be used in production unless absolutely necessary.

### 3. `sysbox-runc` runtime with `CAP_NET_ADMIN`

When using the `sysbox-runc` runtime (from Nestybox), the Boundary can run with only:

- `CAP_NET_ADMIN`

The sysbox-runc runtime provides more complete support for unprivileged user namespaces and nested containerization, which typically eliminates the need for seccomp profile modifications.

## Docker Seccomp Profile Considerations

Docker’s default seccomp profile frequently blocks the `clone` syscall, which is required by the Boundary when creating unprivileged network namespaces. If the `clone` syscall is denied, the Boundary will fail to start.

To address this, you may need to modify or override the seccomp profile used by your container to explicitly allow the required `clone` variants.

You can find the default Docker seccomp profile for your Docker version here (specify your docker version):

https://github.com/moby/moby/blob/v25.0.13/profiles/seccomp/default.json#L628-L635

If the profile blocks the necessary `clone` syscall arguments, you can provide a custom seccomp profile that adds an allow rule like the following:

```json
{
  "names": [
    "clone"
  ],
  "action": "SCMP_ACT_ALLOW"
}
```

This example unblocks the clone syscall entirely.

### Example: Overriding the Docker Seccomp Profile

To use a custom seccomp profile, start by downloading the default profile for your Docker version:

https://github.com/moby/moby/blob/v25.0.13/profiles/seccomp/default.json#L628-L635

Save it locally as seccomp-v25.0.13.json, then insert the clone allow rule shown above (or add "clone" to the list of allowed syscalls).

Once updated, you can run the container with the custom seccomp profile:

```bash
docker run -it \
  --cap-add=NET_ADMIN \
  --security-opt seccomp=seccomp-v25.0.13.json \
  test bash
```

This instructs Docker to load your modified seccomp profile while granting only the minimal required capability (`CAP_NET_ADMIN`).
