# Dev Containers

Dev containers extend your template with containerized development environments,
allowing developers to work in consistent, reproducible setups defined by
`devcontainer.json` files.

Coder's Dev Containers Integration uses the standard `@devcontainers/cli` and
Docker to run containers inside workspaces. You can also attach `coder_app`,
`coder_script`, and `coder_env` resources to dev containers using the
`coder_devcontainer` resource's `subagent_id` attribute.

For setup instructions, see
[Dev Containers Integration](../../integrations/devcontainers/integration.md).

For an alternative approach that doesn't require Docker, see
[Envbuilder](../../integrations/devcontainers/envbuilder/index.md).
