{{- /* Heavily inspired by the Go toolchain formatting. */ -}}
usage: {{.FullUsage}}

{{.Short}}
{{ with .Long}} {{.}} {{ end }}

{{- range $index, $group := optionGroups . }}
{{ with $group.Name }} {{- $group.Name }} Options{{ else -}} Options{{- end -}}:
{{- with $group.Description }} {{- . -}} {{ end }}
    {{- range $index, $option := $group.Options }}
    {{- with $option.Flag }}
    --{{- . -}} {{ end }} {{- with $option.FlagShorthand }}, -{{- . -}} {{ end }}
    {{- end }}
{{- end }}
