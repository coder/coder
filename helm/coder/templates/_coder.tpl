{{/*
Service account to merge into the libcoder template
*/}}
{{- define "coder.serviceaccount" -}}
{{- end -}}

{{/*
Deployment to merge into the libcoder template
*/}}
{{- define "coder.deployment" -}}
spec:
  template:
    spec:
      containers:
      -
{{ include "libcoder.containerspec" (list . "coder.containerspec") | indent 8}}

{{- end -}}

{{/*
ContainerSpec for the Coder container of the Coder deployment
*/}}
{{- define "coder.containerspec" -}}
args:
{{- if .Values.coder.commandArgs }}
  {{- toYaml .Values.coder.commandArgs | nindent 12 }}
{{- else }}
  {{- if .Values.coder.workspaceProxy }}
- wsproxy
  {{- end }}
- server
{{- end }}
env:
- name: CODER_HTTP_ADDRESS
  value: "0.0.0.0:8080"
- name: CODER_PROMETHEUS_ADDRESS
  value: "0.0.0.0:2112"
{{- if .Values.provisionerDaemon.pskSecretName }}
- name: CODER_PROVISIONER_DAEMON_PSK
  valueFrom:
    secretKeyRef:
      name: {{ .Values.provisionerDaemon.pskSecretName | quote }}
      key: psk
{{- end }}
  # Set the default access URL so a `helm apply` works by default.
  # See: https://github.com/coder/coder/issues/5024
{{- $hasAccessURL := false }}
{{- range .Values.coder.env }}
{{- if eq .name "CODER_ACCESS_URL" }}
{{- $hasAccessURL = true }}
{{- end }}
{{- end }}
{{- if not $hasAccessURL }}
- name: CODER_ACCESS_URL
  value: {{ include "coder.defaultAccessURL" . | quote }}
{{- end }}
# Used for inter-pod communication with high-availability.
- name: KUBE_POD_IP
  valueFrom:
    fieldRef:
      fieldPath: status.podIP
- name: CODER_DERP_SERVER_RELAY_URL
  value: "http://$(KUBE_POD_IP):8080"
{{- include "coder.tlsEnv" . }}
{{- with .Values.coder.env }}
{{ toYaml . }}
{{- end }}
ports:
- name: "http"
  containerPort: 8080
  protocol: TCP
  {{- if eq (include "coder.tlsEnabled" .) "true" }}
- name: "https"
  containerPort: 8443
  protocol: TCP
  {{- end }}
  {{- range .Values.coder.env }}
  {{- if eq .name "CODER_PROMETHEUS_ENABLE" }}
  {{/*
    This sadly has to be nested to avoid evaluating the second part
    of the condition too early and potentially getting type errors if
    the value is not a string (like a `valueFrom`). We do not support
    `valueFrom` for this env var specifically.
    */}}
  {{- if eq .value "true" }}
- name: "prometheus-http"
  containerPort: 2112
  protocol: TCP
  {{- end }}
  {{- end }}
  {{- end }}
readinessProbe:
  httpGet:
    path: /healthz
    port: "http"
    scheme: "HTTP"
livenessProbe:
  httpGet:
    path: /healthz
    port: "http"
    scheme: "HTTP"
{{- end }}
