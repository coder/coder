# Reference Architecture: up to 1,000 users

The 1,000 users architecture is designed to cover a wide range of workflows.
Examples of subjects that might utilize this architecture include medium-sized
tech startups, educational units, or small to mid-sized enterprises.

**Target load**: API: up to 180 RPS

**High Availability**: non-essential for small deployments

## Hardware recommendations

### Coderd nodes

| Users       | Node capacity       | Replicas            | GCP             | AWS        | Azure             |
| ----------- | ------------------- | ------------------- | --------------- | ---------- | ----------------- |
| Up to 1,000 | 2 vCPU, 8 GB memory | 1-2 / 1 coderd each | `n1-standard-2` | `t3.large` | `Standard_D2s_v3` |

**Footnotes**:

- For small deployments (ca. 100 users, 10 concurrent workspace builds), it is
  acceptable to deploy provisioners on `coderd` nodes.

### Provisioner nodes

| Users       | Node capacity        | Replicas                       | GCP              | AWS          | Azure             |
| ----------- | -------------------- | ------------------------------ | ---------------- | ------------ | ----------------- |
| Up to 1,000 | 8 vCPU, 32 GB memory | 2 nodes / 30 provisioners each | `t2d-standard-8` | `t3.2xlarge` | `Standard_D8s_v3` |

**Footnotes**:

- An external provisioner is deployed as Kubernetes pod.

### Workspace nodes

| Users       | Node capacity        | Replicas                | GCP              | AWS          | Azure             |
| ----------- | -------------------- | ----------------------- | ---------------- | ------------ | ----------------- |
| Up to 1,000 | 8 vCPU, 32 GB memory | 64 / 16 workspaces each | `t2d-standard-8` | `t3.2xlarge` | `Standard_D8s_v3` |

**Footnotes**:

- Assumed that a workspace user needs at minimum 2 GB memory to perform. We
  recommend against over-provisioning memory for developer workloads, as this my
  lead to OOMKiller invocations.
- Maximum number of Kubernetes workspace pods per node: 256

### Database nodes

| Users       | Node capacity       | Replicas | Storage | GCP                | AWS           | Azure             |
| ----------- | ------------------- | -------- | ------- | ------------------ | ------------- | ----------------- |
| Up to 1,000 | 2 vCPU, 8 GB memory | 1        | 512 GB  | `db-custom-2-7680` | `db.t3.large` | `Standard_D2s_v3` |
