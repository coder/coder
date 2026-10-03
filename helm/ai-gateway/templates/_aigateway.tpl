{{/*
Service account to merge into the libcoder template. The Gateway never needs
the Kubernetes API, so the token is not mounted.
*/}}
{{- define "coder-ai-gateway.serviceaccount" -}}
automountServiceAccountToken: false
{{- end }}

{{/*
HTTP probe shared by startup, liveness, and readiness configuration.
*/}}
{{- define "coder-ai-gateway.probe" -}}
httpGet:
  path: {{ .path }}
  port: http
  scheme: {{ .scheme }}
initialDelaySeconds: {{ .probe.initialDelaySeconds }}
{{- range $field := list "periodSeconds" "timeoutSeconds" "successThreshold" "failureThreshold" }}
{{- if hasKey $.probe $field }}
{{ $field }}: {{ index $.probe $field }}
{{- end }}
{{- end }}
{{- end }}

{{/*
Component annotation for pod metadata.
*/}}
{{- define "coder.componentAnnotation" -}}
app.kubernetes.io/component: ai-gateway
{{- end }}

{{/*
Deployment to merge into the libcoder template.
*/}}
{{- define "coder-ai-gateway.deployment" -}}
spec:
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 0
      maxSurge: 1
  template:
    spec:
      automountServiceAccountToken: false
      terminationGracePeriodSeconds: {{ .Values.aigateway.terminationGracePeriodSeconds }}
      containers:
      -
{{ include "libcoder.containerspec" (list . "coder-ai-gateway.containerspec") | indent 8 }}
      volumes:
        {{- if .Values.aigateway.keySecret.name }}
        - name: ai-gateway-auth
          secret:
            secretName: {{ .Values.aigateway.keySecret.name }}
            items:
              - key: {{ .Values.aigateway.keySecret.key }}
                path: key
        {{- end }}
        {{- if .Values.aigateway.listenerTLS.name }}
        - name: ai-gateway-listener
          secret:
            secretName: {{ .Values.aigateway.listenerTLS.name }}
            items:
              - key: {{ .Values.aigateway.listenerTLS.certKey }}
                path: tls.crt
              - key: {{ .Values.aigateway.listenerTLS.keyKey }}
                path: tls.key
        {{- end }}
        {{- if .Values.aigateway.coderTLS.caSecret.name }}
        - name: coder-client-ca
          secret:
            secretName: {{ .Values.aigateway.coderTLS.caSecret.name }}
            items:
              - key: {{ .Values.aigateway.coderTLS.caSecret.key }}
                path: ca.crt
        {{- end }}
        {{- if .Values.aigateway.coderTLS.clientSecret.name }}
        - name: coder-client-tls
          secret:
            secretName: {{ .Values.aigateway.coderTLS.clientSecret.name }}
            items:
              - key: {{ .Values.aigateway.coderTLS.clientSecret.certKey }}
                path: tls.crt
              - key: {{ .Values.aigateway.coderTLS.clientSecret.keyKey }}
                path: tls.key
        {{- end }}
        {{- include "coder.volumeList" . | nindent 8 }}
{{- end }}

{{/*
ContainerSpec for the AI Gateway container of the deployment.
*/}}
{{- define "coder-ai-gateway.containerspec" -}}
args:
- ai-gateway
- start
{{- with .Values.coder.envFrom }}
envFrom:
{{ toYaml . }}
{{- end }}
env:
{{ include "coder-ai-gateway.defaultEnv" . }}
{{/*
User additions follow chart defaults so they may reference or override them.
*/}}
{{- with .Values.coder.env }}
{{ toYaml . }}
{{- end }}
ports:
- name: http
  containerPort: 4001
  protocol: TCP
- name: metrics
  containerPort: 2112
  protocol: TCP
{{- $scheme := ternary "HTTPS" "HTTP" (not (empty .Values.aigateway.listenerTLS.name)) }}
{{- if .Values.coder.startupProbe.enabled }}
startupProbe:
{{ include "coder-ai-gateway.probe" (dict "probe" .Values.coder.startupProbe "path" "/healthz" "scheme" $scheme) | indent 2 }}
{{- end }}
{{- if .Values.coder.livenessProbe.enabled }}
livenessProbe:
{{ include "coder-ai-gateway.probe" (dict "probe" .Values.coder.livenessProbe "path" "/healthz" "scheme" $scheme) | indent 2 }}
{{- end }}
{{- if .Values.coder.readinessProbe.enabled }}
readinessProbe:
{{ include "coder-ai-gateway.probe" (dict "probe" .Values.coder.readinessProbe "path" "/readyz" "scheme" $scheme) | indent 2 }}
{{- end }}
volumeMounts:
{{- if .Values.aigateway.keySecret.name }}
- name: ai-gateway-auth
  mountPath: /etc/coder/ai-gateway-auth
  readOnly: true
{{- end }}
{{- if .Values.aigateway.listenerTLS.name }}
- name: ai-gateway-listener
  mountPath: /etc/coder/ai-gateway-listener
  readOnly: true
{{- end }}
{{- if .Values.aigateway.coderTLS.caSecret.name }}
- name: coder-client-ca
  mountPath: /etc/coder/coder-client-ca
  readOnly: true
{{- end }}
{{- if .Values.aigateway.coderTLS.clientSecret.name }}
- name: coder-client-tls
  mountPath: /etc/coder/coder-client-tls
  readOnly: true
{{- end }}
{{- include "coder.volumeMountList" . | nindent 0 }}
{{- end }}
