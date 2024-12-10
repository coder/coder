# Integrate Coder with Istio

Use Istio service mesh for your Coder workspace traffic to implement access
controls, encrypt service-to-service communication, and gain visibility into
your workspace network patterns. This guide walks through the required steps to
configure the Istio service mesh for use with Coder.

While Istio is platform-independent, this guide assumes you are leveraging
Kubernetes. Ensure you have a running Kubernetes cluster with both Coder and
Istio installed, and that you have administrative access to configure both
systems. Once you have access to your Coder cluster, apply the following
manifest:

```yaml
apiVersion: networking.istio.io/v1alpha3
kind: EnvoyFilter
metadata:
  name: tailscale-behind-istio-ingress
  namespace: istio-system
spec:
  configPatches:
    - applyTo: NETWORK_FILTER
      match:
        listener:
          filterChain:
            filter:
              name: envoy.filters.network.http_connection_manager
      patch:
        operation: MERGE
        value:
          typed_config:
            "@type": type.googleapis.com/envoy.extensions.filters.network.http_connection_manager.v3.HttpConnectionManager
            upgrade_configs:
              - upgrade_type: derp
```
