# Coder Helm Chart

This directory contains the Helm chart used to deploy Coder onto a Kubernetes
cluster. It contains the minimum required components to run Coder on Kubernetes,
and notably (compared to Coder Classic) does not include a database server.

## Getting Started

> **Warning**: The main branch in this repository does not represent the
> latest release of Coder. Please reference our installation docs for
> instructions on a tagged release.

View
[our docs](https://coder.com/docs/install/server/kubernetes)
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
  # Please keep in mind that you should not set `CODER_HTTP_ADDRESS`,
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
    # For production deployments, we recommend configuring your own GitHub
    # OAuth2 provider and disabling the default one.
    - name: CODER_OAUTH2_GITHUB_DEFAULT_PROVIDER_ENABLE
      value: "false"
  tls:
    secretNames:
      - my-tls-secret-name
```

## Network policy

Set `coder.networkPolicy.enable` to create a Kubernetes `NetworkPolicy` for the
Coder pods. Your CNI must enforce NetworkPolicy for it to have any effect.

```yaml
coder:
  networkPolicy:
    enable: true
    ingress:
      # Optional. If empty, any source can reach the HTTP(S) ports.
      from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ingress-nginx
    # Optional. Restrict who can scrape metrics. If empty, any source can.
    prometheus:
      from:
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: monitoring
    # Open other ports, such as pprof.
    extraIngress:
      - ports:
          - port: 6060
            protocol: TCP
```

Once enabled, ingress is limited to the HTTP(S) ports, Prometheus metrics on port
2112 (disable with `coder.networkPolicy.prometheus.enable`), and ports 8080 and
6222 between Coder replicas for high availability. Egress is unrestricted unless you
set `coder.networkPolicy.egress.enable`. In that case only DNS and your
`extraEgress` rules are allowed, so you must also allow PostgreSQL, the
Kubernetes API, workspaces, and any external services Coder uses.
