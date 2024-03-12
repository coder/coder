# Reference Architecture: up to 1,000 users

The 1,000 users architecture is designed to cover a wide range of workflows.
Examples of subjects that might utilize this architecture include medium-sized
tech startups, educational units, or small to mid-sized enterprises.

**Target load**: API: up to 180 RPS

**High Availability**: non-essential for small deployments

## Hardware recommendations

### Coderd nodes

| Users       | Node capacity       | Replicas | GCP             | AWS        | Azure             |
| ----------- | ------------------- | -------- | --------------- | ---------- | ----------------- |
| Up to 1,000 | 2 vCPU, 8 GB memory | 2        | `n1-standard-2` | `t3.large` | `Standard_D2s_v3` |
