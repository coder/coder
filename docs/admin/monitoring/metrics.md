# Deployment Metrics

Coder exposes many metrics which give insight into the current state of a live
Coder deployment. Our metrics are designed to be consumed by a
[Prometheus server](https://prometheus.io/).

If you don't have an Prometheus server installed, you can follow the Prometheus
[Getting started](https://prometheus.io/docs/prometheus/latest/getting_started/)
guide.

## Setting up metrics

To set up metrics monitoring, please read our
[Prometheus integration guide](../integrations/prometheus.md). The following
links point to relevant sections there.

- [Enable Prometheus metrics](../integrations/prometheus.md#enable-prometheus-metrics)
  in the control plane
- [Enable the Prometheus endpoint in Helm](../integrations/prometheus.md#kubernetes-deployment)
  (Kubernetes users only)
- [Configure Prometheus to scrape Coder metrics](../integrations/prometheus.md#prometheus-configuration)
- [See the list of available metrics](../integrations/prometheus.md#available-metrics)
