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
oc login --token=w4r...04s --server=<cluster-url>
```

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
```

The above values are the Coder defaults. You will need to change these values in
accordance with the applied SCC. Retrieve the project UID range with the following
command:

```console
oc get project coder -o json | jq -r '.metadata.annotations'
{
  "openshift.io/sa.scc.uid-range": "1000680000/10000"
}
```

### 3. Configure the Coder service, connection URLs, and cache values

To establish a connection to PostgreSQL, set the `CODER_PG_CONNECTION_URL` value.
[See our Helm documentation](./kubernetes.md) on configuring the PostgreSQL connection
URL as a secret. Additionally, if accessing Coder over a hostname, set the `CODER_ACCESS_URL`
value.

By default, Coder creates the cache directory in `/home/coder/.cache`. Given the
OpenShift-provided UID and `readOnlyRootFS` security context constraint, the Coder
container does not have permission to write to this directory.
To fix this, you can mount a temporary volume in the pod and set
the `CODER_CACHE_DIRECTORY` environment variable to that location.

Additionally, create the Coder service as a `ClusterIP`. In the next step,
you will create an OpenShift route that points to the service HTTP target port.

```yaml
coder:
  service:
    type: ClusterIP
  env:
    - name: CODER_CACHE_DIRECTORY
      value: /cache
    - name: CODER_PG_CONNECTION_URL
      valueFrom:
        secretKeyRef:
          key: url
          name: coder-db-url
    - name: CODER_ACCESS_URL
      value: "https://coder-example.apps.openshiftapps.com"
  securityContext:
    runAsNonRoot: true
    runAsUser: <project-specific UID>
    runAsGroup: <project-specific GID>
    readOnlyRootFilesystem: true
  volumes:
    - name: "cache"
      emptyDir:
        sizeLimit: 500Mi
  volumeMounts:
    - name: "cache"
      mountPath: "/cache"
      readOnly: false
```

> Note: OpenShift provides a Developer Catalog offering you can use to
> install PostgreSQL into your cluster.

### 4. Create the OpenShift route

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

### 5. Install Coder

You can now install Coder using the values you've set from the above steps. To do
so, run the series of `helm` commands below:

```console
helm repo add coder-v2 https://helm.coder.com/v2
helm repo update
helm install coder coder-v2/coder \
  --namespace coder \
  --values values.yaml
```

### 6. Create an OpenShift-compatible image

While the deployment is spinning up, we will need to create some images that
are compatible with OpenShift. These images can then be run without modifying
the Security Context Constraints (SCCs) in OpenShift.

1. Determine the UID range for the project:

   ```console
   oc get project coder -o json | jq -r '.metadata.annotations'
   {
     "openshift.io/description": "",
     "openshift.io/display-name": "coder",
     "openshift.io/requester": "kube:admin",
     "openshift.io/sa.scc.mcs": "s0:c26,c15",
     "openshift.io/sa.scc.supplemental-groups": "1000680000/10000",
     "openshift.io/sa.scc.uid-range": "1000680000/10000"
   }
   ```

   Note the `uid-range` and `supplemental-groups`. In this case, the project `coder`
   has been allocated 10,000 UIDs starting at 1000680000, and 10,000 GIDs starting
   at 1000680000. In this example, we will pick UID and GID 1000680000.

1. Create a `BuildConfig` referencing the source image you want to customize.
   This will automatically kick off a `Build` that will remain pending until step 3.

   > For more information, please consult the [OpenShift Documentation](https://docs.openshift.com/container-platform/4.12/cicd/builds/understanding-buildconfigs.html).

   ```console
   oc create -f - <<EOF
   kind: BuildConfig
   apiVersion: build.openshift.io/v1
   metadata:
     name: enterprise-base
     namespace: coder
   spec:
     output:
       to:
         kind: ImageStreamTag
         name: 'enterprise-base:latest'
     strategy:
       type: Docker
       dockerStrategy:
         imageOptimizationPolicy: SkipLayers
     source:
       type: Dockerfile
       dockerfile: |
         # Specify the source image.
         FROM docker.io/codercom/enterprise-base:ubuntu

         # Switch to root
         USER root

         # As root:
         # 1) Remove the original coder user with UID 1000
         # 2) Add a coder group with an allowed UID
         # 3) Add a coder user as a member of the above group
         # 4) Fix ownership on the user's home directory
         RUN userdel coder && \
             groupadd coder -g 1000680000 && \
             useradd -l -u 1000680000 coder -g 1000680000 && \
             chown -R coder:coder /home/coder

         # Go back to the user 'coder'
         USER coder
     triggers:
       - type: ConfigChange
     runPolicy: Serial
   EOF
   ```

1. Create an `ImageStream` as a target for the previous step:

   ```console
   oc create imagestream enterprise-base
   ```

   The `Build` created in the previous step should now begin.
   Once completed, you should see output similar to the following:

   ```console
   oc get imagestreamtag
   NAME                     IMAGE REFERENCE                                                                                                                                    UPDATED
   enterprise-base:latest   image-registry.openshift-image-registry.svc:5000/coder/enterprise-base@sha256:1dbbe4ee11be9218e1e4741264135a4f57501fe592d94d20db6bfe11692accd1   55 minutes ago
   ```

### 7. Create an OpenShift-compatible template

Start from the default "Kubernetes" template:

```console
echo kubernetes | coderv2 templates init ./openshift-k8s
cd ./openshift-k8s
```

Edit `main.tf` and update the following fields of the Kubernetes pod resource:

- `spec.security_context`: remove this field.
- `spec.container.image`: update this field to the newly built image hosted
  on the OpenShift image registry from the previous step.
- `spec.container.security_context`: remove this field.

Finally, create the template:

```console
coder template create kubernetes -d .
```

This template should be ready to use straight away.
