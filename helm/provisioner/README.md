# Coder Helm Chart

This directory contains the Helm chart used to deploy Coder provisioner daemons onto a Kubernetes
cluster.

External provisioner daemons are an Enterprise feature. Contact sales@coder.com.

## Getting Started

> **Warning**: The main branch in this repository does not represent the
> latest release of Coder. Please reference our installation docs for
> instructions on a tagged release.

View
[our docs](https://coder.com/docs/admin/provisioners)
for detailed installation instructions.

## Values

Please refer to [values.yaml](values.yaml) for available Helm values and their
defaults.

A good starting point for your values file is:

```yaml
coder:
  env:
    - name: CODER_URL
      value: "https://coder.example.com"
    # This env enables the Prometheus metrics endpoint.
    - name: CODER_PROMETHEUS_ADDRESS
      value: "0.0.0.0:2112"
  replicaCount: 10
provisionerDaemon:
  keySecretName: "coder-provisionerd-key"
  keySecretKey: "provisionerd-key"
```

## Specific Examples

Below are some common specific use-cases when deploying a Coder provisioner.

### Set Labels and Annotations

If you need to set deployment- or pod-level labels and annotations, set `coder.{annotations,labels}` or `coder.{podAnnotations,podLabels}`.

Example:

```yaml
coder:
  # ...
  annotations:
    com.coder/annotation/foo: bar
    com.coder/annotation/baz: qux
  labels:
    com.coder/label/foo: bar
    com.coder/label/baz: qux
  podAnnotations:
    com.coder/podAnnotation/foo: bar
    com.coder/podAnnotation/baz: qux
  podLabels:
    com.coder/podLabel/foo: bar
    com.coder/podLabel/baz: qux
```

### Additional Templates

You can include extra Kubernetes manifests in `extraTemplates`.

The below example will also create a `ConfigMap` along with the Helm release:

```yaml
coder:
  # ...
provisionerDaemon:
  # ...
extraTemplates:
  - |
    apiVersion: v1
    kind: ConfigMap
    metadata:
      name: some-config
      namespace: {{ .Release.Namespace }}
    data:
      key: some-value
```

### Disable Service Account Creation

### Deploying multiple provisioners in the same namespace

To deploy multiple provisioners in the same namespace, set the following values explicitly to avoid conflicts:

- `nameOverride`: controls the name of the provisioner deployment
- `serviceAccount.name`: controls the name of the service account.

Note that `nameOverride` does not apply to `extraTemplates`, as illustrated below:

```yaml
coder:
  # ...
  serviceAccount:
    name: other-coder-provisioner
provisionerDaemon:
  # ...
nameOverride: "other-coder-provisioner"
extraTemplates:
	- |
		apiVersion: v1
		kind: ConfigMap
		metadata:
		  name: some-other-config
		  namespace: {{ .Release.Namespace }}
		data:
		  key: some-other-value
```

If you wish to deploy a second provisioner that references an existing service account, you can do so as follows:

- Set `coder.serviceAccount.disableCreate=true` to disable service account creation,
- Set `coder.serviceAccount.workspacePerms=false` to disable creation of a role and role binding,
- Set `coder.serviceAccount.nameOverride` to the name of an existing service account.

See below for a concrete example:

```yaml
coder:
  # ...
  serviceAccount:
    name: preexisting-service-account
    disableCreate: true
    workspacePerms: false
provisionerDaemon:
  # ...
nameOverride: "other-coder-provisioner"
```

## Testing

The test suite for this chart lives in `./tests/chart_test.go`.

Each test case runs `helm template` against the corresponding `test_case.yaml`, and compares the output with that of the corresponding `test_case.golden` in `./tests/testdata`.
If `expectedError` is not empty for that specific test case, no corresponding `.golden` file is required.

To add a new test case:

- Create an appropriately named `.yaml` file in `testdata/` along with a corresponding `.golden` file, if required.
- Add the test case to the array in `chart_test.go`, setting `name` to the name of the files you added previously (without the extension). If appropriate, set `expectedError`.
- Run the tests and ensure that no regressions in existing test cases occur: `go test ./tests`.
