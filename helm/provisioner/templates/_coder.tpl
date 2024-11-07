{{/*
Service account to merge into the libcoder template
*/}}
{{- define "coder.serviceaccount" -}}
{{- end }}

{{/*
Deployment to merge into the libcoder template
*/}}
{{- define "coder.deployment" -}}
spec:
  template:
    spec:
      terminationGracePeriodSeconds: {{ .Values.provisionerDaemon.terminationGracePeriodSeconds }}
      containers:
      -
{{ include "libcoder.containerspec" (list . "coder.containerspec") | indent 8}}

{{- end }}

{{/*
ContainerSpec for the Coder container of the Coder deployment
*/}}
{{- define "coder.containerspec" -}}
args:
{{- if .Values.coder.commandArgs }}
  {{- toYaml .Values.coder.commandArgs | nindent 12 }}
{{- else }}
- provisionerd
- start
{{- end }}
env:
- name: CODER_PROMETHEUS_ADDRESS
  value: "0.0.0.0:2112"
{{- if and (empty .Values.provisionerDaemon.pskSecretName) (empty .Values.provisionerDaemon.keySecretName) }}
{{ fail "Either provisionerDaemon.pskSecretName or provisionerDaemon.keySecretName must be specified." }}
{{- else if and .Values.provisionerDaemon.keySecretName .Values.provisionerDaemon.keySecretKey }}
	{{- if and (not (empty .Values.provisionerDaemon.pskSecretName)) (ne .Values.provisionerDaemon.pskSecretName "coder-provisioner-psk") }}
	{{ fail "Either provisionerDaemon.pskSecretName or provisionerDaemon.keySecretName must be specified, but not both." }}
	{{- else if .Values.provisionerDaemon.tags }}
	{{ fail "provisionerDaemon.tags may not be specified with provisionerDaemon.keySecretName." }}
	{{- end }}
- name: CODER_PROVISIONER_DAEMON_KEY
  valueFrom:
    secretKeyRef:
      name: {{ .Values.provisionerDaemon.keySecretName | quote }}
      key: {{ .Values.provisionerDaemon.keySecretKey | quote }}
{{- else }}
- name: CODER_PROVISIONER_DAEMON_PSK
  valueFrom:
    secretKeyRef:
      name: {{ .Values.provisionerDaemon.pskSecretName | quote }}
      key: psk
{{- end }}
{{- if include "provisioner.tags" . }}
- name: CODER_PROVISIONERD_TAGS
  value: {{ include "provisioner.tags" . }}
{{- end }}
  # Set the default access URL so a `helm apply` works by default.
  # See: https://github.com/coder/coder/issues/5024
{{- $hasAccessURL := false }}
{{- range .Values.coder.env }}
{{- if eq .name "CODER_URL" }}
{{- $hasAccessURL = true }}
{{- end }}
{{- end }}
{{- if not $hasAccessURL }}
- name: CODER_URL
  value: {{ include "coder.defaultAccessURL" . | quote }}
{{- end }}
{{- with .Values.coder.env }}
{{ toYaml . }}
{{- end }}
ports:
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
{{- end }}

{{/*
Convert provisioner tags to the environment variable format
*/}}
{{- define "provisioner.tags" -}}
  {{- $keys := keys .Values.provisionerDaemon.tags | sortAlpha -}}
  {{- range $i, $key := $keys -}}
    {{- $val := get $.Values.provisionerDaemon.tags $key -}}
    {{- if ne $i 0 -}},{{- end -}}{{ $key }}={{ $val }}
  {{- end -}}
{{- end -}}
