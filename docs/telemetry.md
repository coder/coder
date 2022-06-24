# Telemetry

Coder collects telemetry data from all free installations. Our users have the right to know what we collect, why we collect it, and how we use the data.

## What we collect

First of all, we do not collect any information that could threaten the security of your
installation. For example, we do not collect parameters,
environment variables, or passwords.

You can find a full list of the data we collect in the source code [here](https://github.com/coder/coder/blob/main/coderd/telemetry/telemetry.go).

We offer three levels of telemetry that can be configured with the
`CODER_TELEMETRY=x` environment variable. For example, telemetry level 1 can
be configured with `CODER_TELEMETRY=1`.

`CODER_TELEMETRY=2` is our default level. It includes user email and
IP addresses. This information is used in aggregate to understand where our
users are and general demographic information. We may reach out to the
deployment admin, but will never use these emails for outbound marketing.

`CODER_TELEMETRY=1` is our lightweight telemetry setting. It excludes
email and IP addresses, but everything else in the aforementioned source
code is still sent out. In this level, it is nearly impossible for Coder
to associate an installation to specific organizations or users.

`CODER_TELEMETRY=0` disables telemetry altogether. We reserve this setting
for our enterprise customers. You can also reach out to contact@coder.com if
you need a zero-telemetry license due to security policy requirements.

## How we use telemetry

We use telemetry to build product better and faster. Without telemetry, we don't
know which features are most useful, we don't know where users are dropping
off in our funnel, and we don't know if our roadmap is aligned with the
demographics that really use Coder.

Typical SaaS companies collect far more than what we do with little transparency
and configurability. It's hard to imagine our favorite products today existing
without their backers having good intelligence.

We've decided the only way we can make our product open-source _and_ build
at a fast pace is by collecting usage data as well.
