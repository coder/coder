# Deployment logs

To stream kubernetes pods events from the deployment, you can use Coder's [`coder-logstream-kube`](https://github.com/coder/coder-logstream-kube) tool. This can stream logs from the deployment to Coder's workspace startup logs. 

`coder-logstream-kube` can give you useful information about the deployment, such as:

- Easily determine the reason for a pod provision failure, or why a pod is stuck in a pending state.
- Visibility into when pods are OOMKilled, or when they are evicted.
- Filter by namespace, field selector, and label selector to reduce Kubernetes API load.

## Installation

Install the `coder-kubestream-logs` helm chart on the cluster where the deployment is running.

```shell
helm repo add coder-logstream-kube https://helm.coder.com/logstream-kube
helm install coder-logstream-kube coder-logstream-kube/coder-logstream-kube \
    --namespace coder \
    --set url=<your-coder-url-including-http-or-https>
```

## Example logs

Here is an example of the logs you can expect to see in the workspace startup logs:

### Normal pod deployment logs

![normal pod deployment logs](./coder-logstream-kube-logs-normal.png)

### Pod deployment failure logs

![pod deployment failure logs](./coder-logstream-kube-logs-wrong-image.png)

### Pod crash logs

TODO: add screenshot of pod crash logs
