---
display_name: AWS EC2 Multi-Agent Instance Identity
description: Verify AWS instance identity auth for two Coder agents on one EC2 instance
icon: ../../../site/static/icon/aws.svg
maintainer_github: coder
verified: true
tags: [vm, linux, aws, multi-agent, instance-identity]
---

# AWS multi-agent instance identity verification

This template verifies the multi-agent instance-identity authentication flow on
AWS. It provisions a single EC2 instance with two peer root workspace agents,
`main` and `dev`, that both use AWS instance identity authentication.

The key behavior under test is `CODER_AGENT_NAME` disambiguation. Each agent
starts on the same VM with the same EC2 instance identity, but sets a distinct
`CODER_AGENT_NAME` so the Coder server can issue a separate session token for
that specific agent.

## Prerequisites

- AWS credentials configured for Terraform, such as environment variables or an
  attached IAM role.
- A Coder deployment that includes the multi-agent instance-auth changes from
  this branch.
- No special Coder server configuration. AWS instance identity certificates are
  built in.

## What this template creates

- One VPC, subnet, internet gateway, route table, and route table association.
- One security group that allows SSH from anywhere for test access.
- One Ubuntu 24.04 EC2 instance.
- Two Coder agents, `main` and `dev`, on that single EC2 instance.
- Two agent startup flows that set `CODER_AGENT_NAME` before launching the
  corresponding agent init script.

## How to verify

```bash
cd examples/templates/aws-multi-agent
coder templates push verify-multi-agent

coder create test-multi-agent --template verify-multi-agent

coder list
```

After the workspace starts, verify that both agents are connected in the Coder
Dashboard for `test-multi-agent`. You can also connect to each agent directly:

```bash
coder ssh test-multi-agent -a main true
coder ssh test-multi-agent -a dev true
```

## Expected behavior

- Both agents authenticate independently using AWS instance identity.
- Each agent receives its own session token.
- The workspace shows two connected agents in the Coder Dashboard.
- If `CODER_AGENT_NAME` is omitted, the server should return `409 Conflict`
  because the shared instance identity is ambiguous.

## Troubleshooting

- If one agent gets `409 Conflict`, `CODER_AGENT_NAME` is not being set
  correctly for that agent.
- If both agents fail, instance identity authentication is not working. Check
  EC2 metadata service access from the instance.
- Check cloud-init logs with `journalctl -u cloud-init`.
- Check agent logs at `/tmp/coder-agent-main.log` and
  `/tmp/coder-agent-dev.log`.

## Cleanup

```bash
coder delete test-multi-agent
coder templates delete verify-multi-agent
```
