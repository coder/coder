We regularly scale-test Coder with our [scale testing utility](#scaletest-utility). The same utility can be used in your own environment for insights on how Coder scales with your deployment's specific templates, images, etc.

## Recent scale tests

> This section is incomplete. Stay tuned for reference architectures for up to 3,000 users.

| Environment        | Users | Concurrent builds | Concurrent connections (SSH) | Concurrent connections (web) | Last tested  |
| ------------------ | ----- | ----------------- | ---------------------------- | ---------------------------- | ------------ |
| Kubernetes (GKE)   | 1000  | 500               | 10,000                       | 10,000                       | Dec 20, 2022 |
| Docker (Single VM) | 1000  | 500               | 10,000                       | 10,000                       | Dec 20, 2022 |

## Scale testing utility

Since Coder's performance is highly dependent on the templates and workflows you support, we recommend using our scale testing utility against your own environments.

The following command will run the same scenario against your own Coder deployment. You can also specify a template name and any parameter values.

```sh
coder scaletest create-workspaces \
    --count 100 \
    --template "my-custom-template" \
    --parameter image="my-custom-image" \
    --run-command "sleep 2 && echo hello"

# Run `coder scaletest create-workspaces --help` for all usage
```

> To avoid outages and orphaned resources, we recommend running scale tests on a secondary "staging" environment.

The test does the following:

- create `n` workspaces
- establish SSH connection to each workspace
- run `sleep 3 && echo hello` on each workspace via the web terminal
- close connections, attempt to delete all workspaces
- return results (e.g. `99 succeeded, 1 failed to connect`)

Workspace jobs run concurrently, meaning that the test will attempt to connect to each workspace as soon as it is provisioned instead of waiting for all 100 workspaces to create.

## Troubleshooting

If a load test fails or if you are experiencing performance issues during day-to-day use, you can leverage Coder's [performance tracing](#) and [prometheus metrics](../prometheus.md) to identify bottlenecks during scale tests. Additionally, you can use your existing cloud monitoring stack to measure load, view server logs, etc.
