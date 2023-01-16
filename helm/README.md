# Coder Helm Chart

This directory contains the Helm chart used to deploy Coder onto a Kubernetes
cluster. It contains the minimum required components to run Coder on Kubernetes,
and notably (compared to Coder Classic) does not include a database server.

## Getting Started

> **Warning**: The main branch in this repository does not represent the
> latest release of Coder. Please reference our installation docs for
> instructions on a tagged release.

View
[our docs](https://coder.com/docs/coder-oss/latest/install/kubernetes)
for detailed installation instructions.

## Values

Please refer to [values.yaml](values.yaml) for available Helm values and their
defaults.

A good starting point for your values file is:

```yaml
coder:
  # You can specify any environment variables you'd like to pass to Coder
  # here. Coder consumes environment variables listed in
  # `coder server --help`, and these environment variables are also passed
  # to the workspace provisioner (so you can consume them in your Terraform
  # templates for auth keys etc.).
  #
  # Please keep in mind that you should not set `CODER_ADDRESS`,
  # `CODER_TLS_ENABLE`, `CODER_TLS_CERT_FILE` or `CODER_TLS_KEY_FILE` as
  # they are already set by the Helm chart and will cause conflicts.
  env:
    - name: CODER_ACCESS_URL
      value: "https://coder.example.com"
    - name: CODER_PG_CONNECTION_URL
      valueFrom:
        secretKeyRef:
          # You'll need to create a secret called coder-db-url with your
          # Postgres connection URL like:
          # postgres://coder:password@postgres:5432/coder?sslmode=disable
          name: coder-db-url
          key: url

    # This env enables the Prometheus metrics endpoint.
    - name: CODER_PROMETHEUS_ADDRESS
      value: "0.0.0.0:2112"
  tls:
    secretNames:
      - my-tls-secret-name
```
