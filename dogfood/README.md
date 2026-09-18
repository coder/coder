# Dogfood workspace images

This directory holds the build inputs for the images used by Coder's own
dogfood workspaces:

| Path                      | Image                                      |
| ------------------------- | ------------------------------------------ |
| `coder/ubuntu-26.04/`     | `codercom/oss-dogfood:26.04`, `:latest`    |
| `vscode-coder/Dockerfile` | `codercom/oss-dogfood-vscode-coder:latest` |

`.github/workflows/dogfood.yaml` builds each base image, layers the tools from
the repository's `mise.toml` and `mise.lock` with `mise oci build`, validates the
result by running `make gen`, `fmt`, `lint`, and a Linux build inside it, and
pushes to Docker Hub on `main`. Use `coder/Makefile` to build locally.

The workspace templates that consume these images live in the Coder
infrastructure repository, not here. The only contract between the two is the
published image tag, which the templates resolve to a digest. Template changes
therefore go to the infrastructure repository; image changes go here.
