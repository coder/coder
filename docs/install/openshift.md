## Requirements

Before proceeding, please ensure that you have an OpenShift cluster running K8s
1.19+ (OpenShift 4.7+) and have Helm 3.5+ installed. In addition, you'll need to
install the OpenShift CLI (`oc`) to authenticate to your cluster and create
OpenShift resources.

You'll also want to install the
[latest version of Coder](https://github.com/coder/coder/releases/latest)
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

Depending upon your configured Security Context Constraints (SCC), you'll need
to modify some or all of the following `securityContext` values from the default
values:

The below values are modified from Coder defaults and allow the Coder deployment
to run under the SCC `restricted-v2`.

> Note: `readOnlyRootFilesystem: true` is not technically required under
> `restricted-v2`, but is often mandated in OpenShift environments.

```yaml
coder:
  securityContext:
    runAsNonRoot: true # Unchanged from default
    runAsUser: <project-specific UID> # Default: 1000, replace this with the correct UID for your project.
    runAsGroup: <project-specific GID> # Default: 1000, replace this with the correct GID for your project.
    readOnlyRootFilesystem: true # Default: false, this is often required in OpenShift environments.
    seccompProfile: RuntimeDefault # Unchanged from default
```

- For `runAsUser` / `runAsGroup`, you can retrieve the correct values for
  project UID and project GID with the following command:

      ```console
      oc get project coder -o json | jq -r '.metadata.annotations'
      {
        "openshift.io/sa.scc.supplemental-groups": "1000680000/10000",
        "openshift.io/sa.scc.uid-range": "1000680000/10000"
      }
      ```

  Alternatively, you can set these values to `null` to allow OpenShift to
  automatically select the correct value for the project.

- For `readOnlyRootFilesystem`, consult the SCC under which Coder needs to run.
  In the below example, the `restricted-v2` SCC does not require a read-only
  root filesystem, while `restricted-custom` does:

  ```console
  oc get scc -o wide
  NAME               PRIV    CAPS                   SELINUX     RUNASUSER          FSGROUP     SUPGROUP    PRIORITY     READONLYROOTFS   VOLUMES
  restricted-custom   false   ["NET_BIND_SERVICE"]   MustRunAs   MustRunAsRange     MustRunAs   RunAsAny    <no value>   true             ["configMap","downwardAPI","emptyDir","ephemeral","persistentVolumeClaim","projected","secret"]
  restricted-v2       false   ["NET_BIND_SERVICE"]   MustRunAs   MustRunAsRange     MustRunAs   RunAsAny    <no value>   false            ["configMap","downwardAPI","emptyDir","ephemeral","persistentVolumeClaim","projected","secret"]
  ```

  If you are unsure, we recommend setting `readOnlyRootFilesystem` to `true` in
  an OpenShift environment.

- For `seccompProfile`: in some environments, you may need to set this to `null`
  to allow OpenShift to pick its preferred value.

### 3. Configure the Coder service, connection URLs, and cache values

To establish a connection to PostgreSQL, set the `CODER_PG_CONNECTION_URL`
value. [See our Helm documentation](./kubernetes.md) on configuring the
PostgreSQL connection URL as a secret. Additionally, if accessing Coder over a
hostname, set the `CODER_ACCESS_URL` value.

By default, Coder creates the cache directory in `/home/coder/.cache`. Given the
OpenShift-provided UID and `readOnlyRootFS` security context constraint, the
Coder container does not have permission to write to this directory.

To fix this, you can mount a temporary volume in the pod and set the
`CODER_CACHE_DIRECTORY` environment variable to that location. In the below
example, we mount this under `/tmp` and set the cache location to `/tmp/coder`.
This enables Coder to run with `readOnlyRootFilesystem: true`.

> Note: Depending on the number of templates and provisioners you use, you may
> need to increase the size of the volume, as the `coder` pod will be
> automatically restarted when this volume fills up.

Additionally, create the Coder service as a `ClusterIP`. In the next step, you
will create an OpenShift route that points to the service HTTP target port.

```yaml
coder:
  service:
    type: ClusterIP
  env:
    - name: CODER_CACHE_DIRECTORY
      value: /tmp/coder
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
        sizeLimit: 1Gi
  volumeMounts:
    - name: "cache"
      mountPath: "/tmp"
      readOnly: false
```

> Note: OpenShift provides a Developer Catalog offering you can use to install
> PostgreSQL into your cluster.

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

You can now install Coder using the values you've set from the above steps. To
do so, run the series of `helm` commands below:

```console
helm repo add coder-v2 https://helm.coder.com/v2
helm repo update
helm install coder coder-v2/coder \
  --namespace coder \
  --values values.yaml
```

> Note: If the Helm installation fails with a Kubernetes RBAC error, check the
> permissions of your OpenShift user using the `oc auth can-i` command.
>
> The below permissions are the minimum required:
>
> ```console
> oc auth can-i --list
> Resources                                          Non-Resource URLs   Resource Names    Verbs
> selfsubjectaccessreviews.authorization.k8s.io      []                  []                [create]
> selfsubjectrulesreviews.authorization.k8s.io       []                  []                [create]
> *                                                  []                  []                [get list watch create update patch delete deletecollection]
> *.apps                                             []                  []                [get list watch create update patch delete deletecollection]
> *.rbac.authorization.k8s.io                        []                  []                [get list watch create update patch delete deletecollection]
>                                                    [/.well-known/*]    []                [get]
>                                                    [/.well-known]      []                [get]
>                                                    [/api/*]            []                [get]
>                                                    [/api]              []                [get]
>                                                    [/apis/*]           []                [get]
>                                                    [/apis]             []                [get]
>                                                    [/healthz]          []                [get]
>                                                    [/healthz]          []                [get]
>                                                    [/livez]            []                [get]
>                                                    [/livez]            []                [get]
>                                                    [/openapi/*]        []                [get]
>                                                    [/openapi]          []                [get]
>                                                    [/readyz]           []                [get]
>                                                    [/readyz]           []                [get]
>                                                    [/version/]         []                [get]
>                                                    [/version/]         []                [get]
>                                                    [/version]          []                [get]
>                                                    [/version]          []                [get]
> securitycontextconstraints.security.openshift.io   []                  [restricted-v2]   [use]
> ```

### 6. Create an OpenShift-compatible image

While the deployment is spinning up, we will need to create some images that are
compatible with OpenShift. These images can then be run without modifying the
Security Context Constraints (SCCs) in OpenShift.

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

   Note the `uid-range` and `supplemental-groups`. In this case, the project
   `coder` has been allocated 10,000 UIDs and GIDs, both starting at
   `1000680000`.

   In this example, we will pick both UID and GID `1000680000`.

1. Create a `BuildConfig` referencing the source image you want to customize.
   This will automatically kick off a `Build` that will remain pending until
   step 3.

   > For more information, please consult the
   > [OpenShift Documentation](https://docs.openshift.com/container-platform/4.12/cicd/builds/understanding-buildconfigs.html).

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

   The `Build` created in the previous step should now begin. Once completed,
   you should see output similar to the following:

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
- `spec.container.image`: update this field to the newly built image hosted on
  the OpenShift image registry from the previous step.
- `spec.container.security_context`: remove this field.

Finally, create the template:

```console
coder template push kubernetes -d .
```

This template should be ready to use straight away.
