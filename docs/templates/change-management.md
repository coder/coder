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

# Push the new template template version to Coder
coder templates push --yes $CODER_TEMPLATE_NAME \
    --directory $CODER_TEMPLATE_DIR \
    --name=$CODER_TEMPLATE_VERSION # Version name is optional
```

> Looking for an example? See how we push our development image
> and template [via GitHub actions](https://github.com/coder/coder/blob/main/.github/workflows/dogfood.yaml).
