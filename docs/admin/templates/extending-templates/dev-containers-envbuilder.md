# Choose an Approach To Dev Containers

Coder supports two independent ways to run Dev Containers inside a workspace.

Both implement the [Dev Container specification](https://containers.dev/), but they differ in how the container is built,
who controls it, and which runtime requirements exist.

Use this page to decide which path fits your project or platform needs.

## Options at a Glance

| Capability / Trait                       | Dev Containers integration (CLI-based)   | Envbuilder Dev Containers                 |
|------------------------------------------|------------------------------------------|-------------------------------------------|
| Build engine                             | `@devcontainers/cli` + Docker            | Envbuilder transforms the workspace image |
| Runs separate Docker container           | Yes (parent workspace + child container) | No (modifies the parent container)        |
| Multiple Dev Containers per workspace    | Yes                                      | No                                        |
| Rebuild when `devcontainer.json` changes | Yes (auto-prompt)                        | Limited (requires full workspace rebuild) |
| Docker required in workspace             | Yes                                      | No (works in restricted envs)             |
| Templates                                | Standard `devcontainer.json`             | Terraform + Envbuilder blocks             |
| Suitable for CI / AI agents              | Yes. Deterministic, composable           | Less ideal. No isolated container         |

## How To Migrate From Envbuilder to the Dev Containers Integration

1. Ensure the workspace image can run Docker and has sufficient resources:

   ```shell
   docker ps
   ```

1. Remove any Envbuilder blocks that reference `coder_dev_envbuilder` from the template.
1. Add (or keep) a standard `.devcontainer/` folder with `devcontainer.json` in the repository.
1. Follow the [dev containers documentation](./devcontainers.md) for the full list of steps and options.

   At minimum, add the `devcontainers-cli` module and `coder_devcontainer` resource:

   ```terraform
   module "devcontainers_cli" {
     source   = "dev.registry.coder.com/modules/devcontainers-cli/coder"
     agent_id = coder_agent.main.id
   }
   resource "coder_devcontainer" "project" { # `project` in this example is how users will connect to the dev container: `ssh://project.<workspace>.me.coder`
     count            = data.coder_workspace.me.start_count
     agent_id         = coder_agent.main.id
     workspace_folder = "/home/coder/project"
   }
   ```

1. Start a new workspace.
   Coder detects and launches the dev container automatically.
1. Verify ports, SSH, and rebuild prompts function as expected.

## Related Reading

- [Dev Containers Integration](./index.md)
- [Troubleshooting Dev Containers](../../../user-guides/devcontainers/troubleshooting-dev-containers.md)
- [Envbuilder on GitHub](https://github.com/coder/envbuilder)
- [Dev Container specification](https://containers.dev/)
