---
title: Licensing & Usage
---

Coder Agents licensing controls concurrent agent activity and Agent Time usage across your deployment.

## Community licenses

Community licenses support up to five concurrently active agents deployment-wide.
Coder doesn't limit how long those agents can run or how many tasks they complete over time.
Agents queue when more than five agents are active at a time.
With this agent pool, individuals and small teams can experiment with Coder Agents at no cost.

## AI Premium licenses and Agent Time

AI Premium licenses include a customizable amount of Agent Time.
Agent Time is shared across the deployment, allowing unlimited agents to run concurrently while consuming from a shared pool of purchased working hours.
This usage-based model supports enterprise workloads where large development teams, background automation, and API-triggered tasks can create variable bursts of agent activity.

## Agent Time measurement

Agent Time is the cumulative duration of model invocations that produce Coder Agents chat messages.
Coder measures each invocation from immediately before the request to the model provider opens until the response stream is fully consumed.

Agent Time includes:

- Assistant generation steps in top-level and subagent chats.
- Context compaction (summarization) model calls.
- Model-provider tool execution within a model response.
- The time streamed or spent executing tools before an interrupt, kept on the partial assistant messages.

Agent Time excludes:

- Time spent composing or reviewing messages.
- Time between conversation turns while an agent waits for user input.
- Time a parent agent spends waiting for a subagent, because the subagent records its own model invocations.
- Model calls that don't produce chat messages, such as title generation.
- Failed or retried model calls, which persist no content and record no runtime.
- Client-executed tools and chats handed off to external coding agents, since that work happens outside the server.

Coder records Agent Time in milliseconds and sums it across all Coder Agents chats in the deployment.

## Concurrency and usage limits

Coder handles concurrency and usage limits differently depending on the license you use.

### Community concurrency limit

When a Community license deployment reaches its limit of five concurrently active agents, Coder places additional agents in a queue.
When an active agent completes its task, the next queued agent begins its work.

### AI Premium Agent Time exhaustion

Coder sends deployment administrators an in-app soft warning as the deployment approaches its maximum allotted Agent Time, so they can purchase additional Agent Time before the concurrency fallback takes effect.

## Agent Time usage reporting

Coder reports Agent Time usage to Tallyman, a Coder-managed server used for billing and reporting.
Coder sends the total Coder Agents runtime consumed per UTC hour, in milliseconds, with your deployment ID.
Coder doesn't send user-identifiable information or additional chat data to Tallyman.
Coder also shares the reported usage with [Metronome](https://metronome.com), a Stripe product and Coder partner for usage-based billing and reporting.

Your Coder deployment must be able to make outbound HTTPS requests to `https://tallyman-prod.coder.com` to report usage.
Coder attempts to publish usage data approximately every 17 minutes.
You can monitor these requests in `coderd` logs.

A successful request produces a debug log similar to the following example when you enable debug logging with [`CODER_LOG_FILTER=.*`](../../reference/cli/server.md#-l---log-filter):

```sh
[debu] published usage events to tallyman accepted=1 rejected=0
```

Coder sends the license JWT and deployment ID as request headers.
The request body contains one `hb_agent_runtime_v1` event for each UTC hour.
The `runtime_ms` value is the total Agent Time recorded during that hour.
Idle hours have a value of `0`.

The following example reports 1 hour of Agent Time:

```txt
POST /api/v1/events/ingest HTTP/1.1
Host: tallyman-prod.coder.com
Content-Type: application/json
Coder-License-Key: <license-jwt>
Coder-Deployment-ID: 8a4e92f1-3b7c-4d5e-9f12-abc123def456

{
  "events": [
    {
      "id": "hb_agent_runtime_v1:2026-08-18_14:00:00",
      "event_type": "hb_agent_runtime_v1",
      "event_data": {
        "runtime_ms": 3600000
      },
      "created_at": "2026-08-18T14:00:00Z"
    }
  ]
}
```

The event ID contains the start of the UTC hour.
The `created_at` value also identifies the start of that hour, rather than the time when Coder sends the request.
Coder sends raw milliseconds without rounding or converting the value to hours.

A failed request produces a warning similar to the following example:

```sh
[warn] failed to send publish request to tallyman count=1 error="Post \"https://tallyman-prod.coder.com/api/v1/events/ingest\": dial tcp: lookup tallyman-prod.coder.com: no such host"
```

> [!NOTE]
> Air-gapped deployments and deployments with legal restrictions around usage reporting can [contact us](https://coder.com/contact) to discuss alternative methods.
