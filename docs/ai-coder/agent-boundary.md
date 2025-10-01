

Agent Boundaries are process-level firewalls that restrict and audit what autonomous programs, such as AI agents, can access and use. 

[insert screenshot here]


The easiest way to use Agent Boundaries is through existing Coder modules, such as the [Claude Code module](https://registry.coder.com/modules/coder/claude-code). It can also be ran directly in the terminal by installing its [CLI](https://github.com/coder/boundary).

> [!NOTE]
> The Coder Boundary CLI is free and open source. Integrations with the core product, such as through modules, offers strong isolation and is available to Coder Premium customers.

# Supported Agents 

Coder Boundary supports the securing of any terminal-based agent, including your own custom agents. 

# Features

Boundaries extend Coder's trusted workspaces with a defense-in-depth model that detects and prevents destructive actions without reducing productivity by slowing down workflows or blocking automation. They offer the following features:
- Policy-driven access controls: limit what an agent can access (repos, registries, APIs, files, commands)
- Network policy enforcement: block domains, subnets, or HTTP verbs to prevent exfiltration
- Audit-ready: centralize logs, exportable for compliance, with full visibility into agent actions

# Architecture

# Getting Started with Boundary

## Option 1) Apply Boundary through Coder modules

## Option 2) Wrap the agent process with the Boundary CLI
