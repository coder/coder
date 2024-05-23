# Template Change Management

We recommend source-controlling your templates as you would other code. You can
[install Coder](../install/) in CI/CD pipelines to push new template versions.

```console
# Install the Coder CLI
curl -L https://coder.com/install.sh | sh
# curl -L https://coder.com/install.sh | sh -s -- --version=0.x

# To create API tokens, use `coder tokens create`.
# If no `--lifetime` flag is passed during creation, the default token lifetime
# will be 30 days.
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

To cap token lifetime on creation,
[configure Coder server to set a shorter max token lifetime](../cli/server.md#--max-token-lifetime).
For an example, see how we push our development image and template
[with GitHub actions](https://github.com/coder/coder/blob/main/.github/workflows/dogfood.yaml).
