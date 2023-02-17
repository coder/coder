## Requirements

Before proceeding, please ensure that you have an OpenShift cluster running K8s
1.19+ (OpenShift 4.7+) and have Helm 3.5+ installed. In addition, you'll need to
install the OpenShift CLI (`oc`) to authenticate to your cluster and create OpenShift
resources.

You'll also want to install the [latest version of Coder](https://github.com/coder/coder/releases/latest)
locally in order to log in and manage templates.

## Install Coder with OpenShift

### 1. Authenticate to OpenShift and create a Coder project

Run the following command to login to your OpenShift cluster:

```console
oc login <cluster-url>
```

This will configure your local `~/.kube/config` file with the cluster credentials
needed when installing Coder via `helm`.

Next, you will run the below command to create a project for Coder:

```console
oc new-project coder
```

### 2. Configure SecurityContext values

Depending upon your configured Security Context Constraints (SCC), you'll need to set
the following `securityContext` values in the Coder Helm chart:

```yaml
coder:
  securityContext:
    runAsNonRoot: true
    runAsUser: <project-specific UID>
    runAsGroup: <project-specific GID>
    readOnlyRootFilesystem: false
    seccompProfile:
      type: RuntimeDefault
    allowPrivilegeEscalation: false
```

The above values are the Coder defaults. You will need to change these values in
accordance with the applied SCC. To get a current list of SCCs, run the below command:

```console
oc get scc
```

> Note: you must have cluster-admin privileges to manage SCCs

### 3. Set the `CODER_CACHE_DIRECTORY` environment variable

By default, Coder creates the cache directory in `/home/coder/.cache`. Given the
OpenShift-provided UID, the Coder container does not have permission to write to
this directory.

To address this issue, you will need to set the `CODER_CACHE_DIRECTORY` environment
variable in your Helm values to the following:

```yaml
coder:
  env:
    - name: CODER_CACHE_DIRECTORY
      value: /tmp/coder-cache
```

### 4. Set access URL, PostgreSQL connection values

Set the `CODER_PG_CONNECTION_URL` value to enable Coder to establish a connection
to a PostgreSQL instance. [See our Helm documentation](./kubernetes.md) on configuring
the PostgreSQL connection URL as a secret. Additionally, if accessing Coder over a hostname, set
the `CODER_ACCESS_URL` value.

```yaml
coder:
  env:
    - name: CODER_PG_CONNECTION_URL
      valueFrom:
        secretKeyRef:
          key: url
          name: coder-db-url
    - name: CODER_ACCESS_URL
      value: "https://coder-example.apps.openshiftapps.com"
```

### 5. Configure the Coder service

In this step, we will configure the Coder service as a `ClusterIP`, and create an
OpenShift route that points to the service HTTP target port.

> Note that setting the `ClusterIP` service type for Coder is not required.
> `LoadBalancer` and `NodePort` services types can be used.

Below are the Helm chart values for configuring the Coder service as a `ClusterIP`:

```yaml
coder:
  service:
    type: ClusterIP
```

Below is the YAML spec for creating an OpenShift route that sends traffic to the
HTTP port of the Coder service:

```yaml
kind: Route
apiVersion: route.openshift.io/v1
metadata:
  namespace: coder
spec:
  host: https://coder-example.apps.openshiftapps.com
  to:
    kind: Service
    name: coder
  tls:
    # if set to edge, OpenShift will terminate TLS prior to the traffic reaching
    # the service.
    termination: edge
    # if set to Redirect, insecure client connections are redirected to the secure
    # port
    insecureEdgeTerminationPolicy: Redirect
  port:
    targetPort: http
```

Once complete, you can create this route in OpenShift via:

```console
oc apply -f route.yaml
```

### 6. Install Coder

You can now install Coder using the values you've set from the above steps. To do
so, run the series of `helm` commands below:

```console
helm repo add coder-v2 https://helm.coder.com/v2
helm repo update
helm install coder coder-v2/coder \
  --namespace coder \
  --values values.yaml
```
