# Choose an approach to Dev Containers

Coder supports two independent ways to run Dev Containers inside a workspace.

Both implement the [Dev Container specification](https://containers.dev/), but they differ in how the container is built,
who controls it, and which runtime requirements exist.

Use this page to decide which path fits your project or platform needs.

## Options at a glance

| Capability / Trait                       | Dev Containers integration (CLI-based)   | Envbuilder Dev Containers                 |
|------------------------------------------|------------------------------------------|-------------------------------------------|
| Build engine                             | `@devcontainers/cli` + Docker            | Envbuilder transforms the workspace image |
| Runs separate Docker container           | Yes (parent workspace + child container) | No (modifies the parent container)        |
| Multiple Dev Containers per workspace    | Yes                                      | No                                        |
| Rebuild when `devcontainer.json` changes | Yes (auto-prompt)                        | Limited (requires full workspace rebuild) |
| Docker required in workspace             | Yes                                      | No (works in restricted envs)             |
| Admin vs. developer control              | Developer decides per repo               | Platform admin manages via template       |
| Templates                                | Standard `devcontainer.json`             | Terraform + Envbuilder blocks             |
| Suitable for CI / AI agents              | Yes. Deterministic, composable           | Less ideal. No isolated container         |

## When to choose the Dev Containers integration

Choose the new integration if:

- Your workspace image can run Docker (DinD, Sysbox, or a mounted Docker socket).
- You need multiple Dev Containers (like `frontend`, `backend`, `db`) in a single workspace.
- Developers should own their environment and rebuild on demand.
- You rely on features such as automatic port forwarding, full SSH into containers, or change-detection prompts.

[Dev Container integration](./devcontainers.md) documentation.

## When to choose Envbuilder

Envbuilder remains a solid choice when:

- Docker isn’t available or allowed inside the workspace image.
- The platform team wants tight control over container contents via Terraform.
- A single layered environment is sufficient (no need for separate sub-containers).
- You already have Envbuilder templates in production and they meet current needs.

[Envbuilder Dev Container](../managing-templates/devcontainers/add-devcontainer.md#envbuilder-terraform-provider) documentation.

## How to migrate from Envbuilder to the Dev Containers integration

1. Ensure the workspace image can run Docker and has sufficient resources:

   ```shell
   docker ps
   ```

1. Remove any Envbuilder blocks that reference `coder_dev_envbuilder` from the template.
1. Add (or keep) a standard `.devcontainer/` folder with `devcontainer.json` in the repository.
1. Add the `devcontainers-cli` module:

   ```terraform
   module "devcontainers_cli" {
     source   = "dev.registry.coder.com/modules/devcontainers-cli/coder"
     agent_id = coder_agent.main.id
   }
   ```

1. Start a new workspace.
   Coder detects and launches the dev container automatically.
1. Verify ports, SSH, and rebuild prompts function as expected.

## Related reading

- [Dev Containers Integration](./index.md)
- [Troubleshooting Dev Containers](../../../user-guides/devcontainers/troubleshooting-dev-containers.md)
- [Envbuilder on GitHub](https://github.com/coder/envbuilder)
- [Dev Container specification](https://containers.dev/)
