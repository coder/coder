---
title: Licensing & Usage
---

Coder Agents is licensed differently depending on whether your deployment holds a Community license or an AI Premium license with a purchased Agent Time allocation.

## Community licenses

Community licenses support up to five concurrently active agents deployment-wide.
There is no limit on how long those agents can run or how many tasks they complete over time; they simply queue when more than five agents are active.
This enables individuals and small teams to experiment with Coder Agents at no cost.

## AI Premium licenses and Agent Time

AI Premium licenses include a customizable amount of Agent Time.
Agent Time is shared across the deployment, allowing unlimited agents to run concurrently while consuming from a shared pool of purchased working hours.
This usage-based model is designed for enterprise workloads, where large development teams, background automation, and API-triggered tasks can create highly variable bursts of agent activity without being constrained by a concurrency limit.

## How Agent Time is measured

Agent Time is the cumulative duration during which an AI agent is actively processing a task on behalf of the user.
It is measured per interaction step and summed across the duration of a conversation.

**Includes:**

- Large language model inference time
- Tool execution (such as file operations, terminal commands, and workspace provisioning)
- Automated error recovery attempts

**Excludes:**

- Time the customer spends composing or reviewing messages
- Time between conversation turns when the agent is not processing
- Tool execution delegated to external systems outside the agent's direct control

Agent Time does not accrue when a conversation is inactive or awaiting user input.
Agent Time is measured with millisecond precision and is rounded down to the nearest minute for billing purposes.

## Reaching concurrency and usage limits

### Community concurrency limit

When a Community license deployment reaches its limit of five concurrently active agents, any additional agents are placed in a queue.
Queued agents are picked up automatically as soon as capacity frees up, so no work is lost and no action is required from the user.

### AI Premium Agent Time exhaustion

When an AI Premium deployment exhausts its purchased Agent Time, the deployment may be provisioned to fall back to the Community concurrency model until more Agent Time is purchased.
In this state, agents are limited to five concurrent active agents, and any additional agents are queued until capacity is available.

Deployment administrators see an in-app soft warning message as the deployment approaches its maximum allotted Agent Time, so they can purchase additional Agent Time before the concurrency fallback takes effect.
