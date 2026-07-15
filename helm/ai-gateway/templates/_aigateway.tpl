{{/*
Service account to merge into the libcoder template. The Gateway never needs
the Kubernetes API, so the token is not mounted.
*/}}
{{- define "coder-ai-gateway.serviceaccount" -}}
automountServiceAccountToken: false
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
        - name: ai-gateway-auth
          secret:
            secretName: {{ .Values.aigateway.keySecret.name }}
            items:
              - key: {{ .Values.aigateway.keySecret.key }}
                path: key
        {{- if .Values.aigateway.listenerTLS.secretName }}
        - name: ai-gateway-listener
          secret:
            secretName: {{ .Values.aigateway.listenerTLS.secretName }}
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
{{ include "coder-ai-gateway.ownedEnv" . }}
{{/*
User additions follow the chart-owned variables so they may reference
them via $(VAR). Overriding chart-owned names is rejected by validation.
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
{{- $scheme := ternary "HTTPS" "HTTP" (not (empty .Values.aigateway.listenerTLS.secretName)) }}
{{- if .Values.coder.startupProbe.enabled }}
startupProbe:
  httpGet:
    path: /healthz
    port: http
    scheme: {{ $scheme }}
  initialDelaySeconds: {{ .Values.coder.startupProbe.initialDelaySeconds }}
  {{- range $field := list "periodSeconds" "timeoutSeconds" "successThreshold" "failureThreshold" }}
  {{- if hasKey $.Values.coder.startupProbe $field }}
  {{ $field }}: {{ index $.Values.coder.startupProbe $field }}
  {{- end }}
  {{- end }}
{{- end }}
{{- if .Values.coder.livenessProbe.enabled }}
livenessProbe:
  httpGet:
    path: /healthz
    port: http
    scheme: {{ $scheme }}
  initialDelaySeconds: {{ .Values.coder.livenessProbe.initialDelaySeconds }}
  {{- range $field := list "periodSeconds" "timeoutSeconds" "successThreshold" "failureThreshold" }}
  {{- if hasKey $.Values.coder.livenessProbe $field }}
  {{ $field }}: {{ index $.Values.coder.livenessProbe $field }}
  {{- end }}
  {{- end }}
{{- end }}
{{- if .Values.coder.readinessProbe.enabled }}
readinessProbe:
  httpGet:
    path: /readyz
    port: http
    scheme: {{ $scheme }}
  initialDelaySeconds: {{ .Values.coder.readinessProbe.initialDelaySeconds }}
  {{- range $field := list "periodSeconds" "timeoutSeconds" "successThreshold" "failureThreshold" }}
  {{- if hasKey $.Values.coder.readinessProbe $field }}
  {{ $field }}: {{ index $.Values.coder.readinessProbe $field }}
  {{- end }}
  {{- end }}
{{- end }}
volumeMounts:
- name: ai-gateway-auth
  mountPath: /etc/coder/ai-gateway-auth
  readOnly: true
{{- if .Values.aigateway.listenerTLS.secretName }}
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
