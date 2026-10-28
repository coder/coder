---
title: Telemetry
---

> [!NOTE]
> TL;DR: disable telemetry by setting <code>CODER_TELEMETRY_ENABLE=false</code>.

Coder collects telemetry from all installations by default.
We believe our users should have the right to know what we collect, why we collect it, and how we use the data.

## What we collect

You can find a full list of the data we collect in our source code [here](../../../coderd/telemetry/telemetry.go).
In particular, look at the struct types such as `Template` or `Workspace`.

As a rule, we **do not collect** the following types of information:

- Any data that could make your installation less secure
- Any data that could identify individual users, except the administrator.

For example, we do not collect parameters, environment variables, or user email addresses.
We do collect the administrator email.

Workspace agent statistics include session counts keyed by normalized application names, which can include custom IDE names that workspace agents report.

For workspace builds that Agent Relay creates, we read the `agent_relay_*` parameters that Agent Relay sets. We report the agent provider, the client platform, the attempt number, and SHA-256 hashes of the session ID and pool name. We do not collect other parameter values.

## Why we collect

Telemetry helps us understand which features are most valuable, what use cases to focus on, and which bugs to fix first.

Most cloud-based software products collect far more data than we do.
They often offer little transparency and configurability.
It's hard to imagine our favorite SaaS products existing without their creators having a detailed understanding of user interactions.
We want to wield some of that product development power to build self-hosted, open-source software.

## Security

In the event we discover a critical security issue with Coder, we will use telemetry to identify affected installations and notify their administrators.

<a id="toggling"></a>

## Turn telemetry on or off

You can turn telemetry on or off using either the `CODER_TELEMETRY_ENABLE=[true|false]` environment variable or the `--telemetry=[true|false]` command-line flag.
