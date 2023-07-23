# Deployment logs
To stream kubernetes pods events from the deployment, you can use Coder's [`coder-logstream-kube`](https://github.com/coder/coder-logstream-kube) tool. This can stream logs from the deployment to Coder's workspace startup logs. You just need to install the `coder-kubestream-logs` helm chart on the cluster where the deployment is running.

```shell
helm repo add coder-logstream-kube https://helm.coder.com/logstream-kube
helm install coder-logstream-kube coder-logstream-kube/coder-logstream-kube \
    --namespace coder \
    --set url=<your-coder-url-including-http-or-https>
```

