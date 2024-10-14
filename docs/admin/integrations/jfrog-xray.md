# Integrating JFrog Xray with Coder Kubernetes Workspaces

<div>
  <a href="https://github.com/matifali" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Muhammad Atif Ali</span>
    <img src="https://github.com/matifali.png" width="24px" height="24px" style="vertical-align:middle; margin: 0px;"/>
  </a>
</div>
March 17, 2024

---

This guide will walk you through the process of adding
[JFrog Xray](https://jfrog.com/xray/) integration to Coder Kubernetes workspaces
using Coder's [JFrog Xray Integration](https://github.com/coder/coder-xray).

## Prerequisites

- A self-hosted JFrog Platform instance.
- Kubernetes workspaces running on Coder.

## Deploying the Coder - JFrog Xray Integration

1. Create a JFrog Platform
   [Access Token](https://jfrog.com/help/r/jfrog-platform-administration-documentation/access-tokens)
   with a user that has the read
   [permission](https://jfrog.com/help/r/jfrog-platform-administration-documentation/permissions)
   for the repositories you want to scan.
1. Create a Coder [token](../../reference/cli/tokens_create.md#tokens-create)
   with a user that has the [`owner`](../users#roles) role.
1. Create Kubernetes secrets for the JFrog Xray and Coder tokens.

   ```bash
   kubectl create secret generic coder-token --from-literal=coder-token='<token>'
   kubectl create secret generic jfrog-token --from-literal=user='<user>' --from-literal=token='<token>'
   ```

1. Deploy the Coder - JFrog Xray integration.

   ```bash
   helm repo add coder-xray https://helm.coder.com/coder-xray

   helm upgrade --install coder-xray coder-xray/coder-xray \
   --namespace coder-xray \
   --create-namespace \
   --set namespace="<CODER_WORKSPACES_NAMESPACE>" \ # Replace with your Coder workspaces namespace
   --set coder.url="https://<your-coder-url>" \
   --set coder.secretName="coder-token" \
   --set artifactory.url="https://<your-artifactory-url>" \
   --set artifactory.secretName="jfrog-token"
   ```

### Updating the Coder template

[`coder-xray`](https://github.com/coder/coder-xray) will scan all kubernetes
workspaces in the specified namespace. It depends on the `image` available in
Artifactory and indexed by Xray. To ensure that the images are available in
Artifactory, update the Coder template to use the Artifactory registry.

```tf
image = "<ARTIFACTORY_URL>/<REPO>/<IMAGE>:<TAG>"
```

> **Note**: To authenticate with the Artifactory registry, you may need to
> create a
> [Docker config](https://jfrog.com/help/r/jfrog-artifactory-documentation/docker-advanced-topics)
> and use it in the `imagePullSecrets` field of the kubernetes pod. See this
> [guide](../../tutorials/image-pull-secret.md) for more information.

![JFrog Xray Integration](../../images/guides/xray-integration/example.png)
