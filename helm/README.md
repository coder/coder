<!-- DO NOT EDIT. THIS IS GENERATED FROM README.md.gotmpl -->

# Coder Helm Chart

This directory contains the Helm chart used to deploy Coder onto a Kubernetes
cluster. It contains the minimum required components to run Coder on Kubernetes,
and notably (compared to Coder Classic) does not include a database server.

## Getting Started

> ⚠️ **Warning**: The main branch in this repository does not represent the
> latest release of Coder. Please reference our installation docs for
> instructions on a tagged release.

View
[our docs](https://coder.com/docs/coder-oss/latest/install#kubernetes-via-helm)
for detailed installation instructions.

## Values

| Key                 | Type | Description | Default                         |
| ------------------- | ---- | ----------- | ------------------------------- |
| coder | object | Primary configuration for `coder server`. | `{"env":[{"name":"CODER_ACCESS_URL","value":"https://coder.example.com"}],"image":{"pullPolicy":"IfNotPresent","repo":"ghcr.io/coder/coder","tag":""},"resources":{},"service":{"enable":true,"externalTrafficPolicy":"Cluster","loadBalancerIP":"","type":"LoadBalancer"},"tls":{"secretName":""}}` |
| coder.env | list | The environment variables to set for Coder. These can be used to configure all aspects of `coder server`. Please see `coder server --help` for information about what environment variables can be set. Note: The following environment variables are set by default and cannot be overridden: - CODER_ADDRESS: set to 0.0.0.0:80 and cannot be changed. - CODER_TLS_ENABLE: set if tls.secretName is not empty. - CODER_TLS_CERT_FILE: set if tls.secretName is not empty. - CODER_TLS_KEY_FILE: set if tls.secretName is not empty. | `[{"name":"CODER_ACCESS_URL","value":"https://coder.example.com"}]` |
| coder.image | object | The image to use for Coder. | `{"pullPolicy":"IfNotPresent","repo":"ghcr.io/coder/coder","tag":""}` |
| coder.image.pullPolicy | string | The pull policy to use for the image. See: https://kubernetes.io/docs/concepts/containers/images/#image-pull-policy | `"IfNotPresent"` |
| coder.image.repo | string | The repository of the image. | `"ghcr.io/coder/coder"` |
| coder.image.tag | string | The tag of the image, defaults to {{.Chart.AppVersion}} if not set. | `""` |
| coder.resources | object | The resources to request for Coder. These are optional and are not set by default. | `{}` |
| coder.service | object | The Service object to expose for Coder. | `{"enable":true,"externalTrafficPolicy":"Cluster","loadBalancerIP":"","type":"LoadBalancer"}` |
| coder.service.enable | bool | Whether to create the Service object. | `true` |
| coder.service.externalTrafficPolicy | string | The external traffic policy to use. You may need to change this to "Local" to preserve the source IP address in some situations. https://kubernetes.io/docs/tasks/access-application-cluster/create-external-load-balancer/#preserving-the-client-source-ip | `"Cluster"` |
| coder.service.loadBalancerIP | string | The IP address of the LoadBalancer. If not specified, a new IP will be generated each time the load balancer is recreated. It is recommended to manually create a static IP address in your cloud and specify it here in production to avoid accidental IP address changes. | `""` |
| coder.service.type | string | The type of service to expose. See: https://kubernetes.io/docs/concepts/services-networking/service/#publishing-services-service-types | `"LoadBalancer"` |
| coder.tls | object | The TLS configuration for Coder. | `{"secretName":""}` |
| coder.tls.secretName | string | The name of the secret containing the TLS certificate. The secret should exist in the same namespace as the Helm deployment and should be of type "kubernetes.io/tls". The secret will be automatically mounted into the pod if specified, and the correct "CODER_TLS_*" environment variables will be set for you. | `""` |
