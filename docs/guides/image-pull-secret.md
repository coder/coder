# Defining ImagePullSecrets for Coder workspaces

<div>
  <a href="https://github.com/ericpaulsen" style="text-decoration: none; color: inherit;">
    <span style="vertical-align:middle;">Your Name</span>
    <img src="https://github.com/ericpaulsen.png" width="24px" height="24px" style="vertical-align:middle; margin: 0px;"/>
  </a>
</div>
January 12, 2024

---

Coder workspaces are commonly run as Kubernetes pods. When run inside of an
enterprise, the pod image is typically pulled from a private image registry.
This guide walks through creating an ImagePullSecret to use for authenticating
to your registry, and defining it in your workspace template.

## 1. Create Docker Config JSON File

Create a Docker configuration JSON file containing your registry credentials.
Replace `<your-registry>`, `<your-username>`, and `<your-password>` with your
actual Docker registry URL, username, and password.

```json
{
  "auths": {
    "<your-registry>": {
      "username": "<your-username>",
      "password": "<your-password>"
    }
  }
}
```

## 2. Create Kubernetes Secret

Run the below `kubectl` command in the K8s cluster where you intend to run your
Coder workspaces:

```console
kubectl create secret generic regcred \
  --from-file=.dockerconfigjson=<path-to-docker-config.json> \
  --type=kubernetes.io/dockerconfigjson \
  --namespace=<workspaces-namespace>
```

Inspect the secret to confirm its contents:

```console
kubectl get secret -n <workspaces-namespace> regcred --output="jsonpath={.data.\.dockerconfigjson}" | base64 --decode
```

The output should look similar to this:

```json
{
  "auths": {
    "your.private.registry.com": {
      "username": "ericpaulsen",
      "password": "xxxx",
      "auth": "c3R...zE2"
    }
  }
}
```

## 3. Define ImagePullSecret in Terraform template

```hcl
resource "kubernetes_pod" "dev" {
  metadata {
    # this must be the same namespace where workspaces will be deployed
    namespace = "workspaces-namespace"
  }

  spec {
    image_pull_secrets {
      name = "regcred"
    }
    container {
      name  = "dev"
      image = "your-image:latest"
    }
  }
}
```
