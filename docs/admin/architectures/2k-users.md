# Reference Architecture: up to 2,000 users

In the 2,000 users architecture, there is a moderate increase in traffic,
suggesting a growing user base or expanding operations. This setup is
well-suited for mid-sized companies experiencing growth or for universities
seeking to accommodate their expanding user populations.

Users can be evenly distributed between 2 regions or be attached to different
clusters.

**Target load**: API: up to 300 RPS

**High Availability**: The mode is _disabled_, but administrators may consider
enabling it for deployment reliability.

## Hardware recommendations

### Coderd nodes

| Users       | Node capacity        | Replicas | GCP             | AWS         | Azure             |
| ----------- | -------------------- | -------- | --------------- | ----------- | ----------------- |
| Up to 2,000 | 4 vCPU, 16 GB memory | 2        | `n1-standard-4` | `t3.xlarge` | `Standard_D4s_v3` |

### Workspace nodes

| Users       | Node capacity        | Replicas | GCP              | AWS          | Azure             |
| ----------- | -------------------- | -------- | ---------------- | ------------ | ----------------- |
| Up to 2,000 | 8 vCPU, 32 GB memory | 2        | `t2d-standard-8` | `t3.2xlarge` | `Standard_D8s_v3` |

TODO

Max pods per node 256

Developers for up to 2000+ users architecture are in 2 regions (a different
cluster) and are evenly split. In practice, this doesn’t change much besides the
diagram and workspaces node pool autoscaling config as it still uses the central
provisioner. Recommend multiple provisioner groups for zero-trust and
multi-cloud use cases.

### Provisioner nodes

TODO

For example, to support 120 concurrent workspace builds:

- Create a cluster/nodepool with 4 nodes, 8-core each (AWS: `t3.2xlarge` GCP:
  `e2-highcpu-8`)
- Run coderd with 4 replicas, 30 provisioner daemons each.
  (`CODER_PROVISIONER_DAEMONS=30`)
- Ensure Coder's [PostgreSQL server](./configure.md#postgresql-database) can use
  up to 2 cores and 4 GB RAM
