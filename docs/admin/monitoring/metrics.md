---
title: Deployment metrics
---

Coder exposes Prometheus metrics that show the current state of a live Coder deployment.
This page helps administrators find the metrics for each part of a deployment and turn them on.

If you don't have a Prometheus server, follow the Prometheus [Getting started](https://prometheus.io/docs/prometheus/latest/getting_started/) guide.

## Set up metrics

The [Prometheus integration guide](../integrations/prometheus.md) covers setup for the control plane:

- [Enable Prometheus metrics](../integrations/prometheus.md#enable-prometheus-metrics) in the control plane.
- [Enable the Prometheus endpoint in Helm](../integrations/prometheus.md#kubernetes-deployment) if you deploy on Kubernetes.
- [Configure Prometheus to scrape Coder metrics](../integrations/prometheus.md#prometheus-configuration).

Some components run as separate processes and serve metrics on their own endpoints.
Turn on and scrape each of them separately.
The pages in the next section explain how.

## Metrics by area

Each of these pages lists the metrics for one part of a deployment and explains where to collect them:

| Area                  | Metrics                                                                                                    | Separate endpoint |
|-----------------------|------------------------------------------------------------------------------------------------------------|-------------------|
| External provisioners | [Provisioner metrics](../../install/operate/provisioners/index.md#metrics)                                 | Yes               |
| Workspace proxies     | [Workspace proxy observability](../networking/workspace-proxies.md#observability)                          | Yes               |
| Prebuilt workspaces   | [Prebuilt workspace metrics](../../templates/extending-templates/prebuilt-workspaces.md#available-metrics) | No                |
| AI Gateway            | [AI Gateway Prometheus metrics](../../ai-governance/ai-gateway/monitoring.md#prometheus-metrics)           | Standalone only   |
| Agent Relay           | [Agent Relay Prometheus metrics](../../agent-relay/monitoring/index.md#prometheus-metrics)                 | Yes               |

## All metrics

For every metric Coder exports, with its type, description, and labels, refer to [Available metrics](../integrations/prometheus.md#available-metrics).
