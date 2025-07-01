# envbuilder dogfood template

This template uses the same image as the [dogfood](../dogfood) template, but
builds it on-demand using the latest _preview_ version of [envbuilder](https://github.com/coder/envbuilder).

In theory, it should work with any Git repository containing a `devcontainer.json`.
The Git repository specified by `devcontainer_repo` is cloned into `/workspaces` upon startup and the container is built from the devcontainer located under the path specified by `devcontainer_dir`.
The `region` parameters are the same as for the [dogfood](../dogfood) template.

The `/workspaces` directory is persisted as a Docker volume, so any changes you make to the dogfood Dockerfile or devcontainer.json will be applied upon restarting your workspace.

## Personalization

The startup script runs your `~/personalize` file if it exists.
You also have a persistent home directory under `/home/coder`.
