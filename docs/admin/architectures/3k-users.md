# Reference Architecture: up to 3,000 users

The 3,000 users architecture targets large-scale enterprises, possibly with
on-premises network and cloud deployments.

**Target load**: API: up to 550 RPS

**High Availability**: Typically, such scale requires a fully-managed HA
PostgreSQL service, and all Coder observability features enabled for operational
purposes.

## Hardware recommendations

### Coderd nodes

| Users       | Node capacity        | Replicas | GCP             | AWS         | Azure             |
| ----------- | -------------------- | -------- | --------------- | ----------- | ----------------- |
| Up to 3,000 | 8 vCPU, 32 GB memory | 4        | `n1-standard-4` | `t3.xlarge` | `Standard_D4s_v3` |

### Workspace nodes

TODO

Developers for up to 3000+ users architecture are also in an on-premises
network. Document a provisioner running in a different cloud environment, and
the zero-trust benefits of that.
