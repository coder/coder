# Reference Architecture: up to 3,000 users

The 3,000 users architecture targets large-scale enterprises, possibly with
on-premises network and cloud deployments.

**Target load**: API: up to 550 RPS

**High Availability**: Typically, such scale requires a fully-managed HA
PostgreSQL service, and all Coder observability features enabled for operational
purposes.

**Observability**: Deploy monitoring solutions to gather Prometheus metrics and
visualize them with Grafana to gain detailed insights into infrastructure and
application behavior. This allows operators to respond quickly to incidents and
continuously improve the reliability and performance of the platform.

## Hardware recommendations

### Coderd nodes

| Users       | Node capacity        | Replicas          | GCP             | AWS         | Azure             |
| ----------- | -------------------- | ----------------- | --------------- | ----------- | ----------------- |
| Up to 3,000 | 8 vCPU, 32 GB memory | 4 / 1 coderd each | `n1-standard-4` | `t3.xlarge` | `Standard_D4s_v3` |

### Provisioner nodes

| Users       | Node capacity        | Replicas                 | GCP              | AWS          | Azure             |
| ----------- | -------------------- | ------------------------ | ---------------- | ------------ | ----------------- |
| Up to 3,000 | 8 vCPU, 32 GB memory | 8 / 30 provisioners each | `t2d-standard-8` | `t3.2xlarge` | `Standard_D8s_v3` |

**Footnotes**:

- An external provisioner is deployed as Kubernetes pod.
- It is strongly discouraged to run provisioner daemons on `coderd` nodes at
  this level of scale.
- Separate provisioners into different namespaces in favor of zero-trust or
  multi-cloud deployments.

### Workspace nodes

| Users       | Node capacity        | Replicas                       | GCP              | AWS          | Azure             |
| ----------- | -------------------- | ------------------------------ | ---------------- | ------------ | ----------------- |
| Up to 3,000 | 8 vCPU, 32 GB memory | 256 nodes / 12 workspaces each | `t2d-standard-8` | `t3.2xlarge` | `Standard_D8s_v3` |

**Footnotes**:

- Assumed that a workspace user needs 2 GB memory to perform
- Maximum number of Kubernetes workspace pods per node: 256
- As workspace nodes can be distributed between regions, on-premises networks
  and cloud areas, consider different namespaces in favor of zero-trust or
  multi-cloud deployments.

### Database nodes

| Users       | Node capacity        | Replicas | Storage | GCP                 | AWS             | Azure             |
| ----------- | -------------------- | -------- | ------- | ------------------- | --------------- | ----------------- |
| Up to 3,000 | 8 vCPU, 32 GB memory | 2        | 1.5 TB  | `db-custom-8-30720` | `db.t3.2xlarge` | `Standard_D8s_v3` |

**Footnotes**:

- Consider adding more replicas if the workspace activity is higher than 1500
  workspace builds per day or to achieve higher RPS.
