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

!!!!! DO NOT ADD ANY MORE SELECTORS. IT IS A BREAKING CHANGE !!!!!
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
app.kubernetes.io/part-of: {{ include "coder.name" . }}
{{- if .Chart.AppVersion }}
app.kubernetes.io/version: {{ .Chart.AppVersion | quote }}
{{- end }}
app.kubernetes.io/managed-by: {{ .Release.Service }}
{{- end }}

{{/*
Coder Docker image URI
*/}}
{{- define "coder.image" -}}
{{- if and (eq .Values.coder.image.tag "") (eq .Chart.AppVersion "0.1.0") -}}
{{ fail "You must specify the coder.image.tag value if you're installing the Helm chart directly from Git." }}
{{- end -}}
{{ .Values.coder.image.repo }}:{{ .Values.coder.image.tag | default (printf "v%v" .Chart.AppVersion) }}
{{- end }}

{{/*
Coder TLS enabled.
*/}}
{{- define "coder.tlsEnabled" -}}
    {{- if hasKey .Values.coder "tls" -}}
      {{- if .Values.coder.tls.secretNames -}}
        true
      {{- else -}}
        false
      {{- end -}}
    {{- else -}}
      false
    {{- end -}}
{{- end }}

{{/*
Coder TLS environment variables.
*/}}
{{- define "coder.tlsEnv" }}
{{- if eq (include "coder.tlsEnabled" .) "true" }}
- name: CODER_TLS_ENABLE
  value: "true"
- name: CODER_TLS_ADDRESS
  value: "0.0.0.0:8443"
- name: CODER_TLS_CERT_FILE
  value: "{{ range $idx, $secretName := .Values.coder.tls.secretNames -}}{{ if $idx }},{{ end }}/etc/ssl/certs/coder/{{ $secretName }}/tls.crt{{- end }}"
- name: CODER_TLS_KEY_FILE
  value: "{{ range $idx, $secretName := .Values.coder.tls.secretNames -}}{{ if $idx }},{{ end }}/etc/ssl/certs/coder/{{ $secretName }}/tls.key{{- end }}"
{{- end }}
{{- end }}

{{/*
Coder default access URL
*/}}
{{- define "coder.defaultAccessURL" }}
{{- if eq (include "coder.tlsEnabled" .) "true" -}}
https
{{- else -}}
http
{{- end -}}
://coder.{{ .Release.Namespace }}.svc.cluster.local
{{- end }}

{{/*
Coder volume definitions.
*/}}
{{- define "coder.volumeList" }}
{{- if hasKey .Values.coder "tls" -}}
{{- range $secretName := .Values.coder.tls.secretNames }}
- name: "tls-{{ $secretName }}"
  secret:
    secretName: {{ $secretName | quote }}
{{ end -}}
{{- end }}
{{ range $secret := .Values.coder.certs.secrets -}}
- name: "ca-cert-{{ $secret.name }}"
  secret:
    secretName: {{ $secret.name | quote }}
{{ end -}}
{{ if gt (len .Values.coder.volumes) 0 -}}
{{ toYaml .Values.coder.volumes }}
{{ end -}}
{{- end }}

{{/*
Coder volumes yaml.
*/}}
{{- define "coder.volumes" }}
{{- if trim (include "coder.volumeList" .) -}}
volumes:
{{- include "coder.volumeList" . -}}
{{- else -}}
volumes: []
{{- end -}}
{{- end }}

{{/*
Coder volume mounts.
*/}}
{{- define "coder.volumeMountList" }}
{{- if hasKey .Values.coder "tls" }}
{{ range $secretName := .Values.coder.tls.secretNames -}}
- name: "tls-{{ $secretName }}"
  mountPath: "/etc/ssl/certs/coder/{{ $secretName }}"
  readOnly: true
{{ end -}}
{{- end }}
{{ range $secret := .Values.coder.certs.secrets -}}
- name: "ca-cert-{{ $secret.name }}"
  mountPath: "/etc/ssl/certs/{{ $secret.name }}.crt"
  subPath: {{ $secret.key | quote }}
  readOnly: true
{{ end -}}
{{ if gt (len .Values.coder.volumeMounts) 0 -}}
{{ toYaml .Values.coder.volumeMounts }}
{{ end -}}
{{- end }}

{{/*
Coder volume mounts yaml.
*/}}
{{- define "coder.volumeMounts" }}
{{- if trim (include "coder.volumeMountList" .) -}}
volumeMounts:
{{- include "coder.volumeMountList" . -}}
{{- else -}}
volumeMounts: []
{{- end -}}
{{- end }}

{{/*
Coder ingress wildcard hostname with the wildcard suffix stripped.
*/}}
{{- define "coder.ingressWildcardHost" -}}
{{/* This regex replace is required as the original input including the suffix
   * is not a legal ingress host. We need to remove the suffix and keep the
   * wildcard '*'.
   *
   *   - '\\*'     Starts with '*'
   *   - '[^.]*'   Suffix is 0 or more characters, '-suffix'
   *   - '('       Start domain capture group
   *   -   '\\.'     The domain should be separated with a '.' from the subdomain
   *   -   '.*'      Rest of the domain.
   *   - ')'       $1 is the ''.example.com'
   */}}
{{- regexReplaceAll "\\*[^.]*(\\..*)" .Values.coder.ingress.wildcardHost "*${1}" -}}
{{- end }}

{{/*
Fail on fully deprecated values or deprecated value combinations. This is
included at the top of coder.yaml.
*/}}
{{- define "coder.verifyDeprecated" }}
{{/*
Deprecated value coder.tls.secretName must not be used.
*/}}
{{- if .Values.coder.tls.secretName }}
{{ fail "coder.tls.secretName is deprecated, use coder.tls.secretNames instead." }}
{{- end }}
{{- end }}

{{/*
Renders a value that contains a template.
Usage:
{{ include "coder.renderTemplate" ( dict "value" .Values.path.to.the.Value "context" $) }}
*/}}
{{- define "coder.renderTemplate" -}}
    {{- if typeIs "string" .value }}
        {{- tpl .value .context }}
    {{- else }}
        {{- tpl (.value | toYaml) .context }}
    {{- end }}
{{- end -}}
