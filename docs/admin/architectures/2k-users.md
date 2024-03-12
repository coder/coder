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
