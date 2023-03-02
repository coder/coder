# Template Change Management

We recommend source controlling your templates as you would other code. [Install Coder](../install/) in CI/CD pipelines to push new template versions.

```console
# Install the Coder CLI
curl -L https://coder.com/install.sh | sh
# curl -L https://coder.com/install.sh | sh -s -- --version=0.x

# To create API tokens, use `coder tokens create`.
# These variables are consumed by Coder
export CODER_URL=https://coder.example.com
export CODER_SESSION_TOKEN=*****

# Template details
export CODER_TEMPLATE_NAME=kubernetes
export CODER_TEMPLATE_DIR=.coder/templates/kubernetes
export CODER_TEMPLATE_VERSION=$(git rev-parse --short HEAD)

# Push the new template version to Coder
coder templates push --yes $CODER_TEMPLATE_NAME \
    --directory $CODER_TEMPLATE_DIR \
    --name=$CODER_TEMPLATE_VERSION # Version name is optional
```

> Looking for an example? See how we push our development image
> and template [via GitHub actions](https://github.com/coder/coder/blob/main/.github/workflows/dogfood.yaml).

> You need to set `CODER_MAX_TOKEN_LIFETIME` to a higher value on your coder server to allow creating tokens with a more than 1 month life.
>, e.g., for a system install, you can set `CODER_MAX_TOKEN_LIFETIME=8760h0m0s` in `/etc/coder/coder.env` to allow max token life of 1 year.
