# Coder Helm Chart

This directory contains the Helm chart used to deploy Coder provisioner daemons onto a Kubernetes
cluster.

External provisioner daemons are an Enterprise feature. Contact sales@coder.com.

## Getting Started

> **Warning**: The main branch in this repository does not represent the
> latest release of Coder. Please reference our installation docs for
> instructions on a tagged release.

View
[our docs](https://coder.com/docs/admin/provisioners)
for detailed installation instructions.

## Values

Please refer to [values.yaml](values.yaml) for available Helm values and their
defaults.

A good starting point for your values file is:

```yaml
coder:
  env:
    - name: CODER_URL
      value: "https://coder.example.com"
    # This env enables the Prometheus metrics endpoint.
    - name: CODER_PROMETHEUS_ADDRESS
      value: "0.0.0.0:2112"
  replicaCount: 10
provisionerDaemon:
  pskSecretName: "coder-provisioner-psk"
```
