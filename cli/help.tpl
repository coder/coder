{{- /* Heavily inspired by the Go toolchain formatting. */ -}}
Usage: {{.FullUsage}}

{{ with .Short }}
{{- wrapTTY . }}
{{"\n"}}
{{- end}}
{{- with .Long}}
{{- formatLong . }}
{{- end }}
{{ " " }}
{{- range $index, $group := optionGroups . }}
{{ with $group.Name }} {{- print $group.Name " Options" | prettyHeader }} {{ else -}} {{ prettyHeader "Options"}}{{- end -}}
{{- with $group.Description }}
{{ formatGroupDescription . }}
{{- else }}
{{ " " }}
{{- end }}
    {{- range $index, $option := $group.Options }}
    {{- with flagName $option }}
    --{{- . -}} {{ end }} {{- with $option.FlagShorthand }}, -{{- . -}} {{ end }}
    {{- with envName $option }}, ${{ . }} {{ end }}
    {{- with $option.Default }} (default: {{.}}) {{ end }}
        {{- with $option.Description }}
            {{- $desc := wrapTTY $option.Description }}
{{ indent $desc 2}}
{{- if isDeprecated $option }} DEPRECATED {{ end }}
        {{- end -}}
    {{- end }}
{{- end }}
{{- range $index, $child := .Children }}
{{- if eq $index 0 }}
{{ prettyHeader "Subcommands"}}
{{- end }}
    {{ indent $child.Name 1 | trimNewline }}{{"\t"}}{{ indent $child.Short 1 | trimNewline }}
{{- end }}
