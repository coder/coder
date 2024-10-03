# Telemetry

<blockquote class="info">
TL;DR: disable telemetry by setting <code>CODER_TELEMETRY=false</code>.
</blockquote>

Coder collects telemetry from all installations by default. We believe our users
should have the right to know what we collect, why we collect it, and how we use
the data.

## What we collect

You can find a full list of the data we collect in our source code
[here](https://github.com/coder/coder/blob/main/coderd/telemetry/telemetry.go).
In particular, look at the struct types such as `Template` or `Workspace`.

As a rule, we **do not collect** the following types of information:

- Any data that could make your installation less secure
- Any data that could identify individual users

For example, we do not collect parameters, environment variables, or user email
addresses.

## Why we collect

Telemetry helps us understand which features are most valuable, what use cases
to focus on, and which bugs to fix first.

Most cloud-based software products collect far more data than we do. They often
offer little transparency and configurability. It's hard to imagine our favorite
SaaS products existing without their creators having a detailed understanding of
user interactions. We want to wield some of that product development power to
build self-hosted, open-source software.

## Security

In the event we discover a critical security issue with Coder, we will use
telemetry to identify affected installations and notify their administrators.

## Toggling

You can turn telemetry on or off using either the `CODER_TELEMETRY=[true|false]`
environment variable or the `--telemetry=[true|false]` command-line flag.
