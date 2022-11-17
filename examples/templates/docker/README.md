---
name: Develop in Docker
description: Run workspaces on a Docker host using registry images
tags: [local, docker]
---

# docker

To get started, run `coder templates init`. When prompted, select this template.
Follow the on-screen instructions to proceed.

## Editing the image

Edit the `Dockerfile` and run `coder templates push` to update workspaces.

## Use an existing image

Option 1) Modify the container spec in [main.tf](./main.tf):

```diff
resource "docker_container" "workspace" {
-  image = docker_image.main.name
+  image = "docker.io/codercom/enterprise-base:ubuntu"
```

Option 2) Extend the image in your Dockerfile

```diff
- FROM ubuntu
+ FROM codercom/enterprise-base:ubuntu

+ RUN sudo apt-get update && sudo apt-get install vnc
```

## code-server

`code-server` is installed via the `startup_script` argument in the `coder_agent`
resource block. The `coder_app` resource is defined to access `code-server` through
the dashboard UI over `localhost:13337`.

## Extending this template

See the [kreuzwerker/docker](https://registry.terraform.io/providers/kreuzwerker/docker) Terraform provider documentation to
add the following features to your Coder template:

- SSH/TCP docker host
- Registry authentication
- Build args
- Volume mounts
- Custom container spec
- More

## Troubleshooting

### Agent is stuck "connecting" or "disconnected"

This often occurs because the container cannot reach your [access URL](https://coder.com/docs/coder-oss/latest/admin/configure#access-url). The container may also be missing `curl` which is required to download the agent.

First, check the logs of the container:

```sh
docker ps
# CONTAINER ID   IMAGE                                        COMMAND                  CREATED          STATUS          PORTS     NAMES
# 4334c92f86dd   coder-2a86cbef-b9bd-43b6-b3f2-27bf956016c8   "sh -c '#!/usr/bin/e…"   13 minutes ago   Up 13 minutes             coder-bpmct-base
docker logs 4334c92f86dd
# + status=6
# + echo error: failed to download coder agent
# + echo        command returned: 6
# + echo Trying again in 30 seconds...
# + sleep 30
# + :
# + status=
# + command -v curl
# + curl -fsSL --compressed https://coder.example.ccom/bin/coder-linux-amd64 -o coder
# curl: (6) Could not resolve host: coder.example.ccom
# error: failed to download coder agent
#        command returned: 6
```

> Docker templates typically work with localhost and 127.0.0.1 access URLs as it rewrites to [use the docker host](https://github.com/coder/coder/pull/4306).

In this case, there was a typo in the access URL, which can also be verified in the "Deployment" page. Configure Coder to [use an externally-reachable access URL](https://coder.com/docs/coder-oss/latest/admin/configure#access-url).

![Access URL in deployment settings](https://raw.githubusercontent.com/coder/coder/main/docs/images/admin/deployment-access-url.png)

If you are still running into issues, see our [generic troubleshooting instructions](https://coder.com/docs/coder-oss/latest/templates#troubleshooting-templates) or reach out [on Discord](https://discord.gg/coder).

## How do I persist my files?

With this example, all files within `/home/coder` are persisted when the workspace is started and stopped. You can combine this with a startup script to install software when the workspace starts. See [resource persistance](https://coder.com/docs/coder-oss/latest/templates/resource-persistence) for more details.
