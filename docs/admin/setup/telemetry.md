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

For example, we do not collect environment variables or parameter values, except the Agent Relay fields described below.

For each user account, we collect the following fields:

- A user ID.
- A SHA-256 hash of the email address local part, which is the text before the `@`.
  The domain is sent in plain text.
  The hash is not salted.
- The roles assigned to the user.
- When the account was created.
- The account status.
- The numeric GitHub.com user ID, for users who link a GitHub.com account.
- The login type.

For the first user, meaning the account created earliest on the installation, we also send the full email address in plain text.

Workspace agent statistics include session counts keyed by normalized application names, which can include custom IDE names that workspace agents report.

For start builds of workspaces that Agent Relay creates, we read the `agent_relay_*` parameters that Agent Relay sets. We report the agent provider, the attempt number, and the client platform, lowercased and cut to 15 characters. We also report SHA-256 hashes of the session ID and pool name. The hashes let us count sessions and pools, but they do not hide common pool names such as `default`. We do not collect other parameter values.

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
