# Administration

These guides contain information on managing the Coder control plane and
[authoring templates](./templates/index.md).

First time viewers looking to set up control plane access can start with the
[configuration guide](./setup/index.md). If you're a team lead looking to design
environments for your developers, check out our
[templates guides](./templates/index.md). If you are a developer using Coder, we
recommend the [user guides](../user-guides/index.md).

For automation and scripting workflows, see our [CLI](../reference/cli/index.md)
and [API](../reference/api/index.md) docs.

For any information not strictly contained in these sections, check out our
[Tutorials](../tutorials/index.md) and [FAQs](../tutorials/faqs.md).

## What is an image, template, devcontainer, or workspace

- **Template**
  - [Templates](./templates/index.md) are
	<!-- Managed by Coder Template Administrators The template should include infrastructure-level dependencies for the workspace (for example, Kubernetes PersistentVolumeClaims, docker containers, or EC2 VMs). These should be applicable to all workspaces built from the template. -->

- **Workspace**
  - A [workspace](../user-guides/workspace-management.md) is the environment
    that a developer works in. Developers on a team each work from their own
    workspace and can use [multiple IDEs](./workspace-access/index.md).
- **Image**
  - A [base image](./templates/managing-templates/image-management.md) contains the
    utilities that the Coder workspace is built on. It can be an
    [example image](https://github.com/coder/images), custom image, or one from
    [Docker Hub](https://hub.docker.com/search). It is defined in each template.
		Managed externally to Coder.
		<!-- The devcontainer base image should include dependencies such as the base OS (for example, Debian or Fedora), and OS-level packages (curl, git, java). Include as much as possible here to leverage image and layer caching. Avoid including project-specific tools here. Language-specific runtimes may be added here or in a Dev Container feature. -->

- **Development containers**
  - more about devcontainers...

- **Startup scripts**
  -

<children></children>
