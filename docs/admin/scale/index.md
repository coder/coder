We regularly scale-test Coder against various reference architectures. Additionally, we provide a [scale testing utility](#scaletest-utility) which can be used in your own environment to give insight on how Coder scales with your deployment's specific templates, images, etc.

## Reference Architectures

| Environment                               | Users | Workspaces | Last tested  | Status   |
| ----------------------------------------- | ----- | ---------- | ------------ | -------- |
| [Google Kubernetes Engine (GKE)](#)       | 100   | 200        | Nov 29, 2022 | Complete |
| [AWS Elastic Kubernetes Service (EKS)](#) | 100   | 200        | Nov 29, 2022 | Complete |
| [Google Compute Engine + Docker](#)       | 1000  | 200        | Nov 29, 2022 | Complete |
| [Google Compute Engine + VMs](#)          | 1000  | 200        | Nov 29, 2022 | Complete |

## Scale testing utility

Since Coder's performance is highly dependent on the templates and workflows you support, we recommend using our scale testing utility against your own environments.

For example, this command will do the following:

- create 100 workspaces
- establish a SSH connection to each workspace
- run `sleep 3 && echo hello` on each workspace via the web terminal
- close connections, attempt to delete all workspaces
- return results (e.g. `99 succeeded, 1 failed to connect` )

```sh
coder loadtest create-workspaces \
    --count 100 \
    --template "my-custom-template" \
    --parameter image="my-custom-image" \
    --run-command "sleep 3 && echo hello" \
    --connect-timeout "10s"

# Run `coder scaletest --help` for all usage
```

> To avoid user outages and orphaned resources, we recommend running scale tests on a secondary "staging" environment.

If a test fails, you can leverage Coder's [performance tracing](#) and [prometheus metrics](../prometheus.md) to identify bottlenecks during scale tests. Additionally, you can use your existing cloud monitoring stack to measure load, view server logs, etc.
