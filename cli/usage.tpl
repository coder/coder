{{- /* Heavily inspired by the Go toolchain formatting. */ -}}
usage: {{.FullUsage}}

{{.Short}}
{{ with .Long}} {{.}} {{ end }}

{{- range $index, $group := optionGroups . }}
{{ with $group.Name }} {{- print $group.Name " Options" | prettyHeader }} {{ else -}} {{ prettyHeader "Options"}}{{- end -}}
{{- with $group.Description }}
{{ " " }}
{{- . -}}
{{ end }}
    {{- range $index, $option := $group.Options }}
    {{- with flagName $option }}
    --{{- . -}} {{ end }} {{- with $option.FlagShorthand }}, -{{- . -}} {{ end }}
    {{- with envName $option }}, ${{ . }} {{ end }}
    {{- with $option.Default }} (default: {{.}}) {{ end }}
        {{- with $option.Description }}
            {{- "" }}
            {{- $desc := $option.Description }}
            {{- if isEnterprise $option }} {{$desc = print $desc " Enterprise-Only." }} {{ end }}
{{ $desc := wordWrap $desc 60 -}} {{- indent $desc 2}}
{{- if isDeprecated $option }} DEPRECATED {{ end }}
        {{- end -}}
    {{- end }}
{{- end }}
