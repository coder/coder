# Telemetry

Coder collects telemetry data from all free installations. Our users have the right to know what we collect, why we collect it, and how we use the data.

## What we collect

First of all, we do not collect any information that could threaten the security of your installation. For example, we do not collect parameters, environment variables, or passwords.

You can find a full list of the data we collect in the source code [here](https://github.com/coder/coder/blob/main/coderd/telemetry/telemetry.go).

Telemetry can be configured with the `CODER_TELEMETRY=x` environment variable.

For example, telemetry can be disabled with `CODER_TELEMETRY=false`.

`CODER_TELEMETRY=true` is our default level. It includes user email and IP addresses. This information is used in aggregate to understand where our users are and general demographic information. We may reach out to the deployment admin, but will never use these emails for outbound marketing.

`CODER_TELEMETRY=false` disables telemetry altogether.

## How we use telemetry

We use telemetry to build product better and faster. Without telemetry, we don't know which features are most useful, we don't know where users are dropping off in our funnel, and we don't know if our roadmap is aligned with the demographics that really use Coder.

Typical SaaS companies collect far more than what we do with little transparency and configurability. It's hard to imagine our favorite products today existing without their backers having good intelligence.

We've decided the only way we can make our product open-source _and_ build at a fast pace is by collecting usage data as well.
