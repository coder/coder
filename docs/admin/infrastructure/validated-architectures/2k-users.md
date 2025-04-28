# Reference Architecture: up to 2,000 users

In the 2,000 users architecture, there is a moderate increase in traffic,
suggesting a growing user base or expanding operations. This setup is
well-suited for mid-sized companies experiencing growth or for universities
seeking to accommodate their expanding user populations.

Users can be evenly distributed between 2 regions or be attached to different
clusters.

**Target load**: API: up to 300 RPS

**High Availability**: The mode is _enabled_; multiple replicas provide higher
deployment reliability under load.

## Hardware recommendations

### Coderd nodes

| Users       | Node capacity        | Replicas               | GCP             | AWS         | Azure             |
|-------------|----------------------|------------------------|-----------------|-------------|-------------------|
| Up to 2,000 | 4 vCPU, 16 GB memory | 2 nodes, 1 coderd each | `n1-standard-4` | `m5.xlarge` | `Standard_D4s_v3` |

### Provisioner nodes

| Users       | Node capacity        | Replicas                      | GCP              | AWS          | Azure             |
|-------------|----------------------|-------------------------------|------------------|--------------|-------------------|
| Up to 2,000 | 8 vCPU, 32 GB memory | 4 nodes, 30 provisioners each | `t2d-standard-8` | `c5.2xlarge` | `Standard_D8s_v3` |

**Footnotes**:

- An external provisioner is deployed as Kubernetes pod.
- It is not recommended to run provisioner daemons on `coderd` nodes.
- Consider separating provisioners into different namespaces in favor of
  zero-trust or multi-cloud deployments.

### Workspace nodes

| Users       | Node capacity        | Replicas                      | GCP              | AWS          | Azure             |
|-------------|----------------------|-------------------------------|------------------|--------------|-------------------|
| Up to 2,000 | 8 vCPU, 32 GB memory | 128 nodes, 16 workspaces each | `t2d-standard-8` | `m5.2xlarge` | `Standard_D8s_v3` |

**Footnotes**:

- Assumed that a workspace user needs 2 GB memory to perform
- Maximum number of Kubernetes workspace pods per node: 256
- Nodes can be distributed in 2 regions, not necessarily evenly split, depending
  on developer team sizes

### Database nodes

| Users       | Node capacity        | Replicas | Storage | GCP                 | AWS            | Azure             |
|-------------|----------------------|----------|---------|---------------------|----------------|-------------------|
| Up to 2,000 | 4 vCPU, 16 GB memory | 1 node   | 1 TB    | `db-custom-4-15360` | `db.m5.xlarge` | `Standard_D4s_v3` |

**Footnotes**:

- Consider adding more replicas if the workspace activity is higher than 500
  workspace builds per day or to achieve higher RPS.

**Footnotes for AWS instance types**:

- For production deployments, we recommend using non-burstable instance types,
  such as `m5` or `c5`, instead of burstable instances, such as `t3`.
  Burstable instances can experience significant performance degradation once
  CPU credits are exhausted, leading to poor user experience under sustained load.
