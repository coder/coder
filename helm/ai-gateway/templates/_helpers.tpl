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
Evaluates an extraTemplates entry as a template with the chart context.
Takes a dict with "value" and "context" keys.
*/}}
{{- define "coder-ai-gateway.renderTemplate" -}}
{{- tpl .value .context -}}
{{- end -}}

{{/*
Cross-field validation, invoked once from aigateway.yaml. Emits nothing and
aborts rendering with a specific message on inconsistent values. Each failure
message is asserted verbatim in tests/chart_test.go.
*/}}
{{- define "coder-ai-gateway.validate" -}}
{{- if and (not .Values.coder.image.tag) (eq .Chart.AppVersion "0.1.0") }}
{{- fail "coder.image.tag is required when installing the chart directly from Git." }}
{{- end }}
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
{{- if and .Values.ingress.enable (not .Values.service.enabled) }}
{{- fail "service.enabled must be true when ingress.enable is true." }}
{{- end }}
{{- if and .Values.ingress.enable (not .Values.ingress.host) }}
{{- fail "ingress.host is required when ingress.enable is true." }}
{{- end }}
{{- if and .Values.httproute.enable (not .Values.service.enabled) }}
{{- fail "service.enabled must be true when httproute.enable is true." }}
{{- end }}
{{- if and .Values.httproute.enable (not (.Capabilities.APIVersions.Has "gateway.networking.k8s.io/v1/HTTPRoute")) }}
{{- fail "httproute.enable requires the gateway.networking.k8s.io/v1 HTTPRoute CRD." }}
{{- end }}
{{- $listener := .Values.aigateway.listenerTLS }}
{{- if and $listener.secretName (or (not $listener.certKey) (not $listener.keyKey)) }}
{{- fail "aigateway.listenerTLS.certKey and keyKey are required when secretName is set." }}
{{- end }}
{{- $client := .Values.aigateway.coderTLS.clientSecret }}
{{- if and $client.name (or (not $client.certKey) (not $client.keyKey)) }}
{{- fail "aigateway.coderTLS.clientSecret.certKey and keyKey are required when name is set." }}
{{- end }}
{{- $ca := .Values.aigateway.coderTLS.caSecret }}
{{- if and $ca.name (not $ca.key) }}
{{- fail "aigateway.coderTLS.caSecret.key is required when name is set." }}
{{- end }}
{{/*
Chart-owned variables. Duplicates in env would be ambiguous in Kubernetes.
*/}}
{{- $owned := list "CODER_AI_GATEWAY_HTTP_ADDRESS" "CODER_AI_GATEWAY_KEY_FILE" "CODER_URL" "CODER_PROMETHEUS_ENABLE" "CODER_PROMETHEUS_ADDRESS" "CODER_AI_GATEWAY_TLS_CERT_FILE" "CODER_AI_GATEWAY_TLS_KEY_FILE" "CODER_CLIENT_TLS_CA_FILE" "CODER_CLIENT_TLS_CERT_FILE" "CODER_CLIENT_TLS_KEY_FILE" }}
{{- range .Values.coder.env }}
{{- if has .name $owned }}
{{- fail (printf "coder.env cannot override chart-owned variable %s." .name) }}
{{- end }}
{{- end }}
{{- end -}}
