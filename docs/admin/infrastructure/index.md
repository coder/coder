# Infrastructure

Learn how to spin up & manage Coder infrastructure.

## Architecture

Coder is a self-hosted platform that runs on your own servers. For large
deployments, we recommend running the control plane on Kubernetes. Workspaces
can be run as VMs or Kubernetes pods. The control plane (`coderd`) runs in a
single region. However, workspace proxies, provisioners, and workspaces can run
across regions or even cloud providers for the optimal developer experience.

Learn more about Coder's
[architecture, concepts, and dependencies](./architecture.md).

## Reference Architectures

We publish [reference architectures](./validated-architectures/index.md) that
include best practices around Coder configuration, infrastructure sizing,
autoscaling, and operational readiness for different deployment sizes (e.g.
`Up to 2000 users`).

## Scale Tests

Use our [scale test utility](./scale-utility.md) that can be run on your Coder
deployment to simulate user activity and measure performance.

## Monitoring

See our dedicated [Monitoring](../monitoring/index.md) section for details
around monitoring your Coder deployment via a bundled Grafana dashboard, health
check, and/or within your own observability stack via Prometheus metrics.
