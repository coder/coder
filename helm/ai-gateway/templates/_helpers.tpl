{{/*
CODER_URL value: explicit aigateway.coderURL, or a deterministic in-cluster
Service URL built from aigateway.coderService. The namespace defaults to the
release namespace.
*/}}
{{- define "coder-ai-gateway.coderURL" -}}
{{- if .Values.aigateway.coderURL -}}
{{- .Values.aigateway.coderURL -}}
{{- else -}}
{{- $namespace := default .Release.Namespace .Values.aigateway.coderService.namespace -}}
{{- printf "%s://%s.%s.svc.cluster.local:%v" .Values.aigateway.coderService.scheme .Values.aigateway.coderService.name $namespace .Values.aigateway.coderService.port -}}
{{- end -}}
{{- end -}}

{{/*
Chart-owned environment variables. This is also parsed by validation so the
list of protected names cannot drift from the rendered container environment.
*/}}
{{- define "coder-ai-gateway.ownedEnv" -}}
- name: CODER_AI_GATEWAY_HTTP_ADDRESS
  value: 0.0.0.0:4001
- name: CODER_AI_GATEWAY_KEY_FILE
  value: /etc/coder/ai-gateway-auth/key
- name: CODER_URL
  value: {{ include "coder-ai-gateway.coderURL" . | quote }}
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
{{- if not .Values.aigateway.keySecret.name }}
{{- fail "aigateway.keySecret.name is required." }}
{{- end }}
{{- if not .Values.aigateway.keySecret.key }}
{{- fail "aigateway.keySecret.key is required." }}
{{- end }}
{{- if and .Values.aigateway.coderURL (not (regexMatch "^https?://" .Values.aigateway.coderURL)) }}
{{- fail "aigateway.coderURL must begin with http:// or https://." }}
{{- end }}
{{- if and (not .Values.aigateway.coderURL) (not (has .Values.aigateway.coderService.scheme (list "http" "https"))) }}
{{- fail "aigateway.coderService.scheme must be set to http or https when aigateway.coderURL is empty." }}
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
{{/*
CODER_AI_GATEWAY_KEY conflicts with the chart-owned CODER_AI_GATEWAY_KEY_FILE variable.
Chart always uses CODER_AI_GATEWAY_KEY_FILE, only one can be set.
 */}}
{{- $owned := list "CODER_AI_GATEWAY_KEY" }}
{{- range (include "coder-ai-gateway.ownedEnv" . | fromYamlArray) }}
{{- $owned = append $owned .name }}
{{- end }}
{{- range .Values.coder.env }}
{{- if has .name $owned }}
{{- fail (printf "coder.env cannot override chart-owned variable %s." .name) }}
{{- end }}
{{- end }}
{{- end -}}
