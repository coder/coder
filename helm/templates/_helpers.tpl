{{/*
Expand the name of the chart.
*/}}
{{- define "coder.name" -}}
{{- default .Chart.Name .Values.nameOverride | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Create chart name and version as used by the chart label.
*/}}
{{- define "coder.chart" -}}
{{- printf "%s-%s" .Chart.Name .Chart.Version | replace "+" "_" | trunc 63 | trimSuffix "-" }}
{{- end }}

{{/*
Selector labels
*/}}
{{- define "coder.selectorLabels" -}}
app.kubernetes.io/name: {{ include "coder.name" . }}
app.kubernetes.io/instance: {{ .Release.Name }}
{{- end }}

{{/*
Common labels
*/}}
{{- define "coder.labels" -}}
helm.sh/chart: {{ include "coder.chart" . }}
{{ include "coder.selectorLabels" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Coder listen port (must be > 1024)
*/}}
{{- define "coder.port" }}
{{- if .Values.coder.tls.secretName -}}
8443
{{- else -}}
8080
{{- end -}}
{{- end }}

{{/*
Coder service port
*/}}
{{- define "coder.servicePort" }}
{{- if .Values.coder.tls.secretName -}}
443
{{- else -}}
80
{{- end -}}
{{- end }}

{{/*
Port name
*/}}
{{- define "coder.portName" }}
{{- if .Values.coder.tls.secretName -}}
https
{{- else -}}
http
{{- end -}}
{{- end }}

{{/*
Scheme
*/}}
{{- define "coder.scheme" }}
{{- include "coder.portName" . | upper -}}
{{- end }}
