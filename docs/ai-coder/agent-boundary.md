# Agent Boundary

Agent Boundaries are process-level firewalls that restrict and audit what autonomous programs, such as AI agents, can access and use.

![Screenshot of Agent Boundaries blocking a process](../images/guides/ai-agents/boundary.png)Example of Agent Boundaries blocking a process.

## Supported Agents

Agent Boundaries support the securing of any terminal-based agent, including your own custom agents.

## Features

Agent Boundaries offer network policy enforcement, which blocks domains and HTTP verbs to prevent exfiltration, and writes logs to the workspace.

## Getting Started with Boundary

The easiest way to use Agent Boundaries is through existing Coder modules, such as the [Claude Code module](https://registry.coder.com/modules/coder/claude-code). It can also be ran directly in the terminal by installing the [CLI](https://github.com/coder/boundary).

## Configuration

Boundary is configured using a `config.yaml` file. This allows you to maintain allow lists and share detailed policies with teammates.

In your Terraform module, enable Boundary with minimal configuration:

```tf
module "claude-code" {
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  version             = "4.1.0"
  enable_boundary     = true
  boundary_version    = "v0.2.0"
}
```

Create a `config.yaml` file in your template directory with your policy:

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

### Configuration Parameters

- `boundary_version` defines what version of Boundary is being applied. This is set to `v0.2.0`, which points to the v0.2.0 release tag of `coder/boundary`.
- `log_dir` is the directory where log files are written to when the workspace spins up.
- `log_level` defines the verbosity at which requests are logged. Boundary uses the following verbosity levels:
  - `WARN`: logs only requests that have been blocked by Boundary
  - `INFO`: logs all requests at a high level
  - `DEBUG`: logs all requests in detail
- `proxy_port` defines the port used by the HTTP proxy.
- `allowlist` defines the URLs that the agent can access, in addition to the default URLs required for the agent to work. Rules use the format `"key=value [key=value ...]"`:
  - `domain=github.com` - allows the domain and all its subdomains
  - `domain=*.github.com` - allows only subdomains (the specific domain is excluded)
  - `method=GET,HEAD domain=api.github.com` - allows specific HTTP methods for a domain
  - `method=POST domain=api.example.com path=/users,/posts` - allows specific methods, domain, and paths
  - `path=/api/v1/*,/api/v2/*` - allows specific URL paths

You can also run Agent Boundaries directly in your workspace and configure it per template. You can do so by installing the [binary](https://github.com/coder/boundary) into the workspace image or at start-up. You can do so with the following command:

```hcl
curl -fsSL https://raw.githubusercontent.com/coder/boundary/main/install.sh | bash
 ```

## Jail Types

Boundary supports two different jail types for process isolation, each with different characteristics and requirements:

1. **nsjail** - Uses Linux namespaces for isolation. This is the default jail type and provides network namespace isolation. See [nsjail documentation](./nsjail.md) for detailed information about runtime requirements and Docker configuration.

2. **landjail** - Uses Landlock V4 for network isolation. This provides network isolation through the Landlock Linux Security Module (LSM) without requiring network namespace capabilities. See [landjail documentation](./landjail.md) for implementation details.

The choice of jail type depends on your security requirements, available Linux capabilities, and runtime environment. Both nsjail and landjail provide network isolation, but they use different underlying mechanisms. nsjail uses Linux namespaces, while landjail uses Landlock V4. Landjail may be preferred in environments where namespace capabilities are limited or unavailable.

## Implementation Comparison: Namespaces+iptables vs Landlock V4

| Aspect                        | Current: Namespaces + veth-pair + iptables                                        | Proposed: Landlock V4                                                   |
|-------------------------------|-----------------------------------------------------------------------------------|-------------------------------------------------------------------------|
| **Privileges**                | Requires `CAP_NET_ADMIN`                                                          | ✅ No special capabilities required                                      |
| **Docker seccomp**            | ❌ Requires seccomp profile modifications or sysbox-runc                           | ✅ Works without seccomp changes                                         |
| **Kernel requirements**       | Linux 3.8+ (widely available)                                                     | ❌ Linux 6.7+ (very new, limited adoption)                               |
| **Bypass resistance**         | ✅ Strong - transparent interception prevents bypass                               | ❌ **Medium - can bypass by connecting to `evil.com:<HTTP_PROXY_PORT>`** |
| **Process isolation**         | ✅ PID namespace (processes can't see/kill others); **implementation in-progress** | ❌ No PID namespace (agent can kill other processes)                     |
| **Non-TCP traffic control**   | ✅ Can block/control UDP via iptables; **implementation in-progress**              | ❌ No control over UDP (data can leak via UDP)                           |
| **Application compatibility** | ✅ Works with ANY application (transparent interception)                           | ❌ Tools without `HTTP_PROXY` support will be blocked                    |
