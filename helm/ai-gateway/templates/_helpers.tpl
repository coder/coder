{{/*
Environment variables configured by the chart. User-supplied variables follow
these entries, including when a name is duplicated.
*/}}
{{- define "coder-ai-gateway.defaultEnv" -}}
- name: CODER_AI_GATEWAY_HTTP_ADDRESS
  value: 0.0.0.0:4001
{{- if .Values.aigateway.keySecret.name }}
- name: CODER_AI_GATEWAY_KEY_FILE
  value: /etc/coder/ai-gateway-auth/key
{{- end }}
- name: CODER_PROMETHEUS_ENABLE
  value: "true"
- name: CODER_PROMETHEUS_ADDRESS
  value: 0.0.0.0:2112
{{- if .Values.aigateway.listenerTLS.name }}
- name: CODER_AI_GATEWAY_TLS_CERT_FILE
  value: /etc/coder/ai-gateway-listener/tls.crt
- name: CODER_AI_GATEWAY_TLS_KEY_FILE
  value: /etc/coder/ai-gateway-listener/tls.key
{{- end }}
{{- if .Values.aigateway.coderTLS.caSecret.name }}
- name: CODER_CLIENT_TLS_CA_FILE
  value: /etc/coder/coder-client-ca/ca.crt
{{- end }}
{{- if .Values.aigateway.coderTLS.clientSecret.name }}
- name: CODER_CLIENT_TLS_CERT_FILE
  value: /etc/coder/coder-client-tls/tls.crt
- name: CODER_CLIENT_TLS_KEY_FILE
  value: /etc/coder/coder-client-tls/tls.key
{{- end }}
{{- end -}}

{{/*
Cross-field validation, invoked once from aigateway.yaml. Emits nothing and
aborts rendering with a specific message on inconsistent values. Each failure
message is asserted verbatim in tests/chart_test.go.
*/}}
{{- define "coder-ai-gateway.validate" -}}
{{- if and .Values.aigateway.keySecret.name (not .Values.aigateway.keySecret.key) }}
{{- fail "aigateway.keySecret.key is required when name is set." }}
{{- end }}
{{- if and .Values.ingress.enable (not .Values.service.enable) }}
{{- fail "service.enable must be true when ingress.enable is true." }}
{{- end }}
{{- if and .Values.ingress.enable (not .Values.ingress.host) }}
{{- fail "ingress.host is required when ingress.enable is true." }}
{{- end }}
{{- if and .Values.httproute.enable (not .Values.service.enable) }}
{{- fail "service.enable must be true when httproute.enable is true." }}
{{- end }}
{{- if and .Values.httproute.enable (empty .Values.httproute.parentRefs) }}
{{- fail "httproute.parentRefs is required when httproute.enable is true." }}
{{- end }}
{{- $listener := .Values.aigateway.listenerTLS }}
{{- if and $listener.name (or (not $listener.certKey) (not $listener.keyKey)) }}
{{- fail "aigateway.listenerTLS.certKey and keyKey are required when name is set." }}
{{- end }}
{{- if and .Values.httproute.enable (not (.Capabilities.APIVersions.Has "gateway.networking.k8s.io/v1/HTTPRoute")) }}
{{- fail "httproute.enable requires the gateway.networking.k8s.io/v1 HTTPRoute CRD." }}
{{- end }}
{{- $client := .Values.aigateway.coderTLS.clientSecret }}
{{- if and $client.name (or (not $client.certKey) (not $client.keyKey)) }}
{{- fail "aigateway.coderTLS.clientSecret.certKey and keyKey are required when name is set." }}
{{- end }}
{{- $ca := .Values.aigateway.coderTLS.caSecret }}
{{- if and $ca.name (not $ca.key) }}
{{- fail "aigateway.coderTLS.caSecret.key is required when name is set." }}
{{- end }}
{{- if and .Values.service.nodePort (not (has .Values.service.type (list "NodePort" "LoadBalancer"))) }}
{{- fail "service.nodePort requires service.type to be NodePort or LoadBalancer." }}
{{- end }}
{{- end -}}
