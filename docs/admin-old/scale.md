We scale-test Coder with [a built-in utility](#scale-testing-utility) that can
be used in your environment for insights into how Coder scales with your
infrastructure. For scale-testing Kubernetes clusters we recommend to install
and use the dedicated Coder template,
[scaletest-runner](https://github.com/coder/coder/tree/main/scaletest/templates/scaletest-runner).

Learn more about [Coder’s architecture](../about/architecture.md) and our
[scale-testing methodology](architectures/index.md#scale-testing-methodology).

## Recent scale tests

> Note: the below information is for reference purposes only, and are not
> intended to be used as guidelines for infrastructure sizing. Review the
> [Reference Architectures](architectures/index.md) for hardware sizing
> recommendations.

| Environment      | Coder CPU | Coder RAM | Coder Replicas | Database          | Users | Concurrent builds | Concurrent connections (Terminal/SSH) | Coder Version | Last tested  |
| ---------------- | --------- | --------- | -------------- | ----------------- | ----- | ----------------- | ------------------------------------- | ------------- | ------------ |
| Kubernetes (GKE) | 3 cores   | 12 GB     | 1              | db-f1-micro       | 200   | 3                 | 200 simulated                         | `v0.24.1`     | Jun 26, 2023 |
| Kubernetes (GKE) | 4 cores   | 8 GB      | 1              | db-custom-1-3840  | 1500  | 20                | 1,500 simulated                       | `v0.24.1`     | Jun 27, 2023 |
| Kubernetes (GKE) | 2 cores   | 4 GB      | 1              | db-custom-1-3840  | 500   | 20                | 500 simulated                         | `v0.27.2`     | Jul 27, 2023 |
| Kubernetes (GKE) | 2 cores   | 8 GB      | 2              | db-custom-2-7680  | 1000  | 20                | 1000 simulated                        | `v2.2.1`      | Oct 9, 2023  |
| Kubernetes (GKE) | 4 cores   | 16 GB     | 2              | db-custom-8-30720 | 2000  | 50                | 2000 simulated                        | `v2.8.4`      | Feb 28, 2024 |
| Kubernetes (GKE) | 2 cores   | 4 GB      | 2              | db-custom-2-7680  | 1000  | 50                | 1000 simulated                        | `v2.10.2`     | Apr 26, 2024 |

> Note: a simulated connection reads and writes random data at 40KB/s per
> connection.
