# Agent Boundary

Agent Boundaries are process-level firewalls that restrict and audit what autonomous programs, such as AI agents, can access and use.

![Screenshot of Agent Boundaries blocking a process](../images/guides/ai-agents/boundary.png)Example of Agent Boundaries blocking a process.

## Supported Agents

Agent Boundaries support the securing of any terminal-based agent, including your own custom agents.

## Features

Agent Boundaries offer network policy enforcement, which blocks domains and HTTP verbs to prevent exfiltration, and writes logs to the workspace.

## Getting Started with Boundary

The easiest way to use Agent Boundaries is through existing Coder modules, such as the [Claude Code module](https://registry.coder.com/modules/coder/claude-code). It can also be ran directly in the terminal by installing the [CLI](https://github.com/coder/boundary).

Below is an example of how to configure Agent Boundaries for usage in your workspace.

```tf
module "claude-code" {
  source              = "dev.registry.coder.com/coder/claude-code/coder"
  enable_boundary     = true
  boundary_version    = "main"
  boundary_log_dir    = "/tmp/boundary_logs" 
  boundary_log_level  = "WARN"
  boundary_additional_allowed_urls = ["GET *google.com"]
  boundary_proxy_port = "8087"
  version             = "3.2.1"
}
```

- `boundary_version` defines what version of Boundary is being applied. This is set to `main`, which points to the main branch of `coder/boundary`.
- `boundary_log_dir` is the directory where log files are written to when the workspace spins up.
- `boundary_log_level` defines the verbosity at which requests are logged. Boundary uses the following verbosity levels:
  - `WARN`: logs only requests that have been blocked by Boundary
  - `INFO`: logs all requests at a high level
  - `DEBUG`: logs all requests in detail
- `boundary_additional_allowed_urls`: defines the URLs that the agent can access, in additional to the default URLs required for the agent to work
  - `github.com` means only the specific domain is allowed
  - `*.github.com` means only the subdomains are allowed - the specific domain is excluded
  - `*github.com` means both the specific domain and all subdomains are allowed
  - You can also also filter on methods, hostnames, and paths - for example, `GET,HEAD *github.com/coder`.

You can also run Agent Boundaries directly in your workspace and configure it per template. You can do so by installing the [binary](https://github.com/coder/boundary) into the workspace image or at start-up. You can do so with the following command:

```hcl
curl -fsSL https://raw.githubusercontent.com/coder/boundary/main/install.sh | bash
 ```
