# Choose an Approach To Dev Containers

Coder supports two independent ways to run Dev Containers inside a workspace.

Both implement the [Dev Container specification](https://containers.dev/), but they differ in how the container is built,
who controls it, and which runtime requirements exist.

Use this page to decide which path fits your project or platform needs.

## Options at a Glance

| Capability / Trait                       | Dev Containers integration                 | Envbuilder                                |
|------------------------------------------|--------------------------------------------|-------------------------------------------|
| How it's built                           | `@devcontainers/cli` and Docker            | Envbuilder transforms the workspace image |
| Docker-in-Docker?                        | Yes (parent workspace and child container) | No (modifies the parent container)        |
| Multiple dev containers per workspace    | Yes                                        | No                                        |
| Rebuild when `devcontainer.json` changes | Yes - user-initiated                       | Requires full workspace restart           |

## Related Reading

- [Dev Containers integration](./devcontainers.md)
- [Dev Containers specification](https://containers.dev/)
- [Envbuilder on GitHub](https://github.com/coder/envbuilder)
