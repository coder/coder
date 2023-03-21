{{- /* Heavily inspired by the Go toolchain formatting. */ -}}
Usage: {{.FullUsage}}

{{ with .Short }}
{{- wrapTTY . }}
{{"\n"}}
{{- end}}
{{- with .Long}}
{{- formatLong . }}
{{ "\n" }}
{{- end }}
{{- range $index, $child := visibleChildren . }}
{{- if eq $index 0 }}
{{ prettyHeader "Subcommands"}}
{{- end }}
    {{- "\n" }}
    {{- formatSubcommand . | trimNewline }}
{{- end }}
{{- range $index, $group := optionGroups . }}
{{ with $group.Name }} {{- print $group.Name " Options" | prettyHeader }} {{ else -}} {{ prettyHeader "Options"}}{{- end -}}
{{- with $group.Description }}
{{ formatGroupDescription . }}
{{- else }}
{{- end }}
    {{- range $index, $option := $group.Options }}
	{{- if not (eq $option.FlagShorthand "") }}{{- print "\n  -" $option.FlagShorthand ", " -}}
	{{- else }}{{- print "\n      " -}}
	{{- end }}
    {{- with flagName $option }}--{{ . }}{{ end }}
    {{- with envName $option }}, ${{ . }}{{ end }}
    {{- with $option.Default }} (default: {{ . }}){{ end }}
	{{- with typeHelper $option }} {{ . }}{{ end }}
        {{- with $option.Description }}
            {{- $desc := $option.Description }}
{{ indent $desc 10 }}
{{- if isDeprecated $option }} DEPRECATED {{ end }}
        {{- end -}}
    {{- end }}
{{- end }}
---
{{- if .Parent }}
Run `coder --help` for a list of global options.
{{- else }}
Report bugs and request features at https://github.com/coder/coder/issues/new
{{- end }}
