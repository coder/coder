# Image Management

While Coder provides example
[base container images](https://github.com/coder/images) for
workspaces, it's often best to create custom images that matches the needs of
your users. This document serves a guide to operational maturity with some best
practices around managing workspaces images for Coder.

1. Create a minimal base image
2. Create golden image(s) with standard tooling
3. Create project-specific images for common use cases
4. Allow developers to bring their own images and customizations with Dev
   Containers

An image is just one of the many properties defined within the template.
Templates can pull images from a public image registry (e.g. Docker Hub) or an
internal one, thanks to Terraform.

## Create a minimal base image

While you may not use this directly in Coder templates, it's useful to have a
minimal base image is a small image that contains only the necessary
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

See Coder's
[example base image](https://github.com/coder/images/tree/main/images/minimal)
for reference.

## Create general-purpose golden image(s) with standard tooling

It's often practical to have a few golden images that contain standard tooling
for developers. These images should contain a number of languages (e.g. Python,
Java, TypeScript), IDEs (VS Code, JetBrains, PyCharm), and other tools (e.g.
`docker`). Unlike project-specific images (which are also important), general
purpose images are great for:

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

- [Coder's Universal Image](https://github.com/coder/images/tree/main/images/universal): a catch-all image with many languages and tools preinstalled. Runs as the `coder` user, so it works with Coder templates out of the box. See [coder/images](https://github.com/coder/images) for more example images, including language-specific ones.
- [Universal Dev Containers Image](https://github.com/devcontainers/images/tree/main/src/universal)

## Create project-specific images for common use cases

Beyond golden images, create images scoped to a specific project, language, or
use case (e.g. a Go backend, a Node.js frontend, or a data science stack).
These images stay smaller and faster to pull than a kitchen-sink image, and
give each team exactly the tooling they need.

Examples:

- [coder/images](https://github.com/coder/images): Coder's example
  language-specific images (Go, Java, Node.js, and more), all running as the
  `coder` user
- Rather than baking every tool into the image, you can also layer tools onto
  a smaller image with [mise](https://mise.jdx.dev/). See the
  [install command-line tools guide](../../../get-started/customize-your-template/install-command-line-tools.md)

## Allow developers to bring their own images and customizations with Dev Containers

While golden images are great for general use cases, developers will often need
specific tooling for their projects. The [Dev Container](https://containers.dev)
specification allows developers to define their projects dependencies within a
`devcontainer.json` in their Git repository.

- [Configure a template for Dev Containers](../../integrations/devcontainers/integration.md)
