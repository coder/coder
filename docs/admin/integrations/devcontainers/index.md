# Dev Containers

Dev containers allow developers to define their development environment
as code using the [Dev Container specification](https://containers.dev/).
Configuration lives in a `devcontainer.json` file alongside source code,
enabling consistent, reproducible environments.

By adopting dev containers, organizations can:

- **Standardize environments**: Eliminate "works on my machine" issues while
  still allowing developers to customize their tools within approved boundaries.
- **Scale efficiently**: Let developers maintain their own environment
  definitions, reducing the burden on platform teams.
- **Improve security**: Use hardened base images and controlled package
  registries to enforce security policies while enabling developer self-service.

Coder supports multiple approaches for running dev containers. Choose based on
your infrastructure and workflow requirements.

## Comparison

| Feature                            | Dev Container CLI                                      | Envbuilder                            | CI/CD Pre-built                                           |
|------------------------------------|--------------------------------------------------------|---------------------------------------|-----------------------------------------------------------|
| **Official MS Support**            | ✅ Yes                                                  | ❌ No                                  | ✅ Yes                                                     |
| **Full DevContainer Spec Support** | ✅ All options                                          | ❌ Limited options                     | \~ Most options                                           |
| **Startup Time**                   | Build at runtime, faster with caching                  | Build at runtime, faster with caching | Fast (pre-built)                                          |
| **Docker Required**                | ❌ Yes                                                  | ✅ No                                  | ✅ No                                                      |
| **Caching**                        | More difficult                                         | ✅ Yes                                 | ✅ Yes                                                     |
| **Repo Discovery**                 | ✅ Yes                                                  | ❌ No                                  | ❌ No                                                      |
| **Custom Apps in-spec**            | ✅ Via spec args                                        | ❌ No                                  | ❌ No                                                      |
| **Debugging**                      | Easy                                                   | Very difficult                        | Moderate                                                  |
| **Versioning**                     | \~ Via spec, or template                               | \~ Via spec, or template              | ✅ Image tags                                              |
| **Testing Pipeline**               | \~ Via CLI in CI/CD                                    | \~ Via CLI in CI/CD                   | ✅ Yes, via the same pipeline                              |
| **Feedback Loop**                  | ✅ Fast                                                 | ✅ Fast                                | Slow (build, and then test)                               |
| **Maintenance Status**             | ✅ Active                                               | ⚠️ Maintenance mode                   | ✅ Active                                                  |
| **Best For**                       | Dev flexibility, rapid iteration, feature completeness | Restricted environments               | Controlled and centralized releases, less dev flexibility |

## Dev Container CLI

The Dev Container CLI uses the standard `@devcontainers/cli` and Docker to run
containers inside your workspace. This is the recommended approach for most use
cases and provides the most complete dev container experience.

Uses the
[devcontainers-cli module](https://registry.coder.com/modules/devcontainers-cli),
the `coder_devcontainer` Terraform resource, and
`CODER_AGENT_DEVCONTAINERS_ENABLE=true`.

**Pros:**

- Official Microsoft integration with the `@devcontainers/cli`.
- Supports all dev container configuration options.
- Supports custom arguments in the dev container spec for defining custom apps
  without needing template changes.
- Supports discovery of repos with dev containers in them.
- Easier to debug, since you have access to the outer container.

**Cons / Requirements:**

- Requires Docker in workspaces. This does not necessarily mean Docker-in-Docker
  or a specific Kubernetes runtime — you could use Rootless Podman or a
  privileged sidecar.
- Caching is more difficult. You might want to pair this with the CI/CD Pre-built
  approach to pull the layers. You could also use a shared cache, but that is not
  a great security practice.

**Best for:**

- Dev flexibility, rapid iteration, and feature completeness.
- Workspaces with Docker available (Docker-in-Docker or mounted socket).
- Dev container management in the Coder dashboard (discovery, status, rebuild).
- Multiple dev containers per workspace.

[Configure Dev Containers Integration](./integration.md)

For user documentation, see the
[Dev Containers user guide](../../../user-guides/devcontainers/index.md).

## Envbuilder

Envbuilder transforms the workspace image itself from a `devcontainer.json`,
rather than running containers inside the workspace. It does not require a Docker
daemon.

> [!NOTE]
> Envbuilder is in **maintenance mode**. No new features are planned to be
> implemented. For most use cases, the
> [Dev Container CLI](#dev-container-cli) or [CI/CD Pre-built](#cicd-pre-built)
> approaches are recommended.

**Pros:**

- Does not require Docker in workspaces.
- Easier caching.

**Cons:**

- Very complicated to debug, since Envbuilder replaces the filesystem of the
  container. You can't access that environment within Coder easily if it fails,
  and you won't have many debug tools.
- Does not support all of the dev container configuration options.
- Does not support discovery of repos with dev containers in them.
- Less flexible and more complex in general.

**Best for:**

- Environments where Docker is unavailable or restricted.
- Infrastructure-level control over image builds, caching, and security scanning.
- Kubernetes-native deployments without privileged containers.

[Configure Envbuilder](./envbuilder/index.md)

## CI/CD Pre-built

Build the dev container image from CI/CD and pull it from within Terraform. This
approach separates the image build step from the workspace startup, resulting in
fast startup times and a generic template that doesn't have any
dev container-specific configuration items.

**Pros:**

- Official Microsoft integration.
- Faster startup time — no need for a caching setup.
- The template is generic and doesn't have any dev container-specific
  configuration items.
- Easier caching.
- Versioned via image tags.
- Testable pipeline.

**Cons:**

- Adds a build step.
- Does not support all of the runtime options, but still supports more options
  than Envbuilder.
- Does not support discovery of repos with dev containers.
- Slow feedback loop (build, then test).

**Best for:**

- Controlled and centralized releases with less dev flexibility.
- Teams that already have CI/CD pipelines for building images.
- Environments that need fast, predictable startup times.

For an example workflow, see the
[basic-env CI/CD workflow](https://github.com/uwu/basic-env/blob/main/.github/workflows/_build-and-push.yml).
