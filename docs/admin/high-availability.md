# High Availability

High Availability (HA) mode solves for horizontal scalability and automatic failover
within a single region. When in HA mode, Coder continues using a single Postgres
endpoint. [GCP](https://cloud.google.com/sql/docs/postgres/high-availability), [AWS](https://docs.aws.amazon.com/prescriptive-guidance/latest/saas-multitenant-managed-postgresql/availability.html),
and other cloud vendors offer fully-managed HA Postgres services that pair
nicely with Coder.

For Coder to operate correctly, every node must be within 10ms of each other
and Postgres. We make a best-effort attempt to warn the user when inter-Coder
latency is too high, but if requests start dropping, this is one metric to investigate.
Note that this latency requirement applies _only_ to Coder services. Coder will
operate correctly even with few seconds of latency on
workspace <-> Coder and user <-> Coder connections.

## Setup

> We're stress testing High Availability this week before we enable it by default. To try HA beforehand, set `CODER_EXPERIMENTAL=true` for the Coder server as well as the additional configuration options below.

Coder automatically enters HA mode when multiple instances simultaneously connect
to the same Postgres endpoint.

HA brings one configuration variable to set in each Coder
node: `CODER_DERP_SERVER_RELAY_URL`. The HA nodes use these URLs to communicate
with each other. Inter-node communication is only required while using the
embedded relay (default). If you're using [custom relays](../networking.md#custom-relays), Coder ignores `CODER_DERP_SERVER_RELAY_URL` since Postgres is the sole rendezvous for the Coder nodes.

`CODER_DERP_SERVER_RELAY_URL` will never be `CODER_ACCESS_URL` because
`CODER_ACCESS_URL` is a load balancer to all Coder nodes.

Here's an example 3-node network configuration setup:

| Name      | `CODER_ADDRESS` | `CODER_DERP_SERVER_RELAY_URL` | `CODER_ACCESS_URL`       |
| --------- | --------------- | ----------------------------- | ------------------------ |
| `coder-1` | `*:80`          | `http://10.0.0.1:80`          | `https://coder.big.corp` |
| `coder-2` | `*:80`          | `http://10.0.0.2:80`          | `https://coder.big.corp` |
| `coder-3` | `*:80`          | `http://10.0.0.3:80`          | `https://coder.big.corp` |

## Kubernetes

If you installed Coder via
[our Helm Chart](../install/kubernetes.md#install-coder-with-helm), just
increase `coder.replicaCount` in `values.yaml`.

If you installed Coder into Kubernetes by some other means, insert the relay URL
via the environment like so:

```yaml
env:
  - name: POD_IP
    valueFrom:
      fieldRef:
        fieldPath: status.podIP
  - name: CODER_DERP_SERVER_RELAY_URL
    value: http://$(POD_IP)
```

Then, increase the number of pods.

## Up next

- [Networking](../networking.md)
- [Kubernetes](../install/kubernetes.md)
- [Enterprise](../enterprise.md)
