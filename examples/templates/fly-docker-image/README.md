# Coder Docker Image fly.io Template

This template provisions a [code-server](https://github.com/coder/code-server) instance on [fly.io](https://fly.io) using the [codercom/code-server](https://hub.docker.com/r/codercom/code-server) image.

## Prerequisites

- [flyctl](https://fly.io/docs/getting-started/installing-flyctl/) installed.
- [Coder](https://coder.com/) already setup and running with coder-cli installed locally.

## Deploy

1. Clone this repo and cd into `fly-docker-image` directory.
2. Add a secret or environment variable to your Coder deployment with the name `FLY_API_TOKEN` and the value of your fly.io API token.
   > This is needed to deploy the workspace to fly.io.

```shell
flyctl auth login
export FLY_API_TOKEN=$(flyctl auth token)
```

Add this to your Coder deployment's environment variables. (e.g. `/etc/coder.d/coder.env`)

Or, If Coder is running as a fly.io app, you can set the secret directly:

```shell
flyctl secrets set FLY_API_TOKEN=$(flyctl auth token) -a <coder-app-name>
# where <coder-app-name> is the name of the Coder app on fly.io
```

> Read our blog [post](coder.com/blog/deploying-coder-on-fly-io) to learn more about how to deploy Coder on fly.io.

3. Run `coder templates create fly-docker-image` to create a template in Coder.
4. Create a new workspace from the template.

This is all. You should now have a code-server instance running on fly.io.

> Note: Change the image and the startup command to suit your needs.
