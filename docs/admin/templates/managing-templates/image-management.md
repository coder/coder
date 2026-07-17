# Image Management

While Coder provides example
[container images](https://github.com/coder/images) for
workspaces, it's often best to create custom images that match the needs of
your users. This document serves as a guide to operational maturity with some
best practices around managing workspace images for Coder.

After following this tutorial, you'll accomplish the following:

1. Create a minimal base image.
2. Create golden images with standard tooling.
3. Create project-specific images for common use cases.
4. Let developers customize their own environment.

An image is just one of the many properties defined within the template.
Templates can pull images from a public image registry (e.g. Docker Hub) or an
internal one, thanks to Terraform. Each image reference below can be dropped
directly into a template's image variable or parameter.

## Create a minimal base image

While you may not use this directly in Coder templates, it's useful to have a
minimal base image as a small image that contains only the necessary
dependencies to work in your network and work with Coder. Here are some things
to consider:

- `curl`, `wget`, or `busybox` is required to download and run
  [the agent](../../../../provisionersdk/scripts/bootstrap_linux.sh)
- `git` is recommended so developers can clone repositories
- If the Coder server is using a certificate from an internal certificate
  authority (CA), you'll need to add or mount these into your image
- Other generic utilities that will be required by all users, such as `ssh`,
  `docker`, `bash`, `jq`, and/or internal tooling
- Consider creating (and starting the container with) a non-root user

Examples:

- [`codercom/example-minimal:ubuntu`](https://github.com/coder/images/tree/main/images/minimal): only the necessary dependencies for a Coder workspace to bootstrap
- [`codercom/example-base:ubuntu`](https://github.com/coder/images/tree/main/images/base): a slightly more padded starting point with common utilities preinstalled

## Create golden images with standard tooling

Building on the base image, it's often practical to have a few golden images
that contain standard tooling for developers. These images should contain a
number of languages (e.g. Python, Java, TypeScript), IDEs (VS Code, JetBrains,
PyCharm), and other tools (e.g. `docker`). Unlike project-specific images
(which are also important), general purpose images are great for:

- **Scripting:** Developers may just want to hop in a Coder workspace to run
  basic scripts or queries.
- **Day 1 Onboarding:** New developers can quickly get started with a familiar
  environment without having to browse through (or create) an image
- **Basic Projects:** Developers can use these images for simple projects that
  don't require any specific tooling outside of the standard libraries. As the
  project gets more complex, its best to move to a project-specific image.
- **"Golden Path" Projects:** If your developer platform offers specific tech
  stacks and types of projects, the golden image can be a good starting point
  for those projects.

This is often referred to as a "sandbox" or "kitchen sink" image. Since large
multi-purpose container images can quickly become difficult to maintain, it's
important to keep the number of general-purpose images to a minimum (2-3 in
most cases) with a well-defined scope.

Examples:

- [`codercom/example-universal:ubuntu`](https://github.com/coder/images/tree/main/images/universal): a catch-all image with many languages and tools preinstalled. Runs as the `coder` user, so it works with Coder templates out of the box.
- [`mcr.microsoft.com/devcontainers/universal`](https://github.com/devcontainers/images/tree/main/src/universal): the Universal Dev Containers image

## Create project-specific images for common use cases

Beyond golden images, create images scoped to a specific project, language, or
use case (e.g., a Go backend, a Node.js frontend, or a data science stack).
These images stay smaller and faster to pull than one larger image that tries to install all dependencies.

Examples:

- [`codercom/example-golang:ubuntu`](https://github.com/coder/images/tree/main/images/golang),
  [`codercom/example-java:ubuntu`](https://github.com/coder/images/tree/main/images/java),
  [`codercom/example-node:ubuntu`](https://github.com/coder/images/tree/main/images/node):
  Coder's example language-specific images, all running as the `coder` user.
  Refer to [coder/images](https://github.com/coder/images) for the full list.
- [`codercom/oss-dogfood:latest`](https://github.com/coder/coder/tree/main/dogfood):
  the image Coder's own engineers use to develop Coder. A good reference for a
  project-specific image tailored to a specific team or monorepo setup.

## Let developers customize their own environment

Even with well-scoped images, developers will often need tooling that is
specific to their project or personal workflow. Instead of maintaining an
image for every combination, developers can layer their own customizations
on top of a smaller image:

- [Dev Containers](https://containers.dev): developers define their project's
  dependencies in a `devcontainer.json` in their Git repository, and Coder
  builds the environment on top of your base image. Visit
  [configure a template for Dev Containers](../../integrations/devcontainers/integration.md).
- [mise](https://mise.jdx.dev/): developers install and pin language runtimes
  and CLI tools at workspace startup without rebuilding the image. Visit the
  [install command-line tools guide](../../../get-started/customize-your-template/install-command-line-tools.md).
