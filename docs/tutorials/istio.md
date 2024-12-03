# Configure Istio Service Mesh

Integrating Istio's service mesh with Coder's Ingress enables powerful traffic management, security, and observability capabilities. By placing Coder's workspace traffic behind Istio's intelligent proxy layer, you can implement access controls, encrypt service-to-service communication, and gain visibility into your workspace network patterns. This guide walks through the process of configuring Istio alongside Coder's existing ingress controller, ensuring that developer workspaces remain accessible while benefiting from Istio's comprehensive service mesh features.

Before proceeding, ensure you have a running Kubernetes cluster with both Coder and Istio installed, and that you have administrative access to configure both systems. Once you have access to your Coder cluster, apply the following manifest:

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
