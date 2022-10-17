# High Availability

High Availability (HA) mode solves for horizontal scalability and automatic failover
within a single region. When in HA mode, Coder continues using a single Postgres
endpoint. [GCP](https://cloud.google.com/sql/docs/postgres/high-availability), [AWS](https://docs.aws.amazon.com/prescriptive-guidance/latest/saas-multitenant-managed-postgresql/availability.html), and others offer fully-managed HA Postgres services.

For Coder to operate correctly, all Coder servers must be within 10ms of each other
and Postgres. We make a best-effort attempt to warn the user when inter-coder
latency is too high, but if requests start dropping, this is one metric to investigate.

## Automatic Setup

Coder automatically enters HA mode when multiple instances connect to the same
Postgres endpoint. Thus, enabling HA is as simple as increasing the number
of deployed Coder replicas.

## Kubernetes Setup

- Using our Helm, just increase `coder.replicaCount` in `values.yaml`
- Custom Helm Chart:
  ```
  env:
    - name: POD_IP
      valueFrom:
        fieldRef:
          fieldPath: status.podIP
    - name: CODER_DERP_SERVER_RELAY_URL
      value: http://$(POD_IP)
  ```

## Virtual Machine Setup

Set `CODER_DERP_SERVER_RELAY_URL` to an address that other instances can access:

## Up next

- [Networking](../networking.md)
- [Kubernetes](../install/kubernetes.md.md)
- [Enterprise](./enterprise.md)
