{{- /* Heavily inspired by the Go toolchain and fd */ -}}
coder {{version}}

{{prettyHeader "Usage"}}
{{indent .FullUsage 2}}


{{ with .Short }}
{{- indent . 2 | wrapTTY }}
{{"\n"}}
{{- end}}

{{ with .Aliases }}
{{"  Aliases: "}} {{- joinStrings .}}
{{- end }}

{{- with .Long}}
{{"\n"}}
{{- indent . 2}}
{{ "\n" }}
{{- end }}
{{ with visibleChildren . }}
{{- range $index, $child := . }}
{{- if eq $index 0 }}
{{ prettyHeader "Subcommands"}}
{{- end }}
    {{- "\n" }}
    {{- formatSubcommand . | trimNewline }}
{{- end }}
{{- "\n" }}
{{- end }}
{{- range $index, $group := optionGroups . }}
{{ with $group.Name }} {{- print $group.Name " Options" | prettyHeader }} {{ else -}} {{ prettyHeader "Options"}}{{- end -}}
{{- with $group.Description }}
{{ formatGroupDescription . }}
{{- else }}
{{- end }}
    {{- range $index, $option := $group.Options }}
	{{- if not (eq $option.FlagShorthand "") }}{{- print "\n "}} {{ keyword "-"}}{{keyword $option.FlagShorthand }}{{", "}}
	{{- else }}{{- print "\n      " -}}
	{{- end }}
    {{- with flagName $option }}{{keyword "--"}}{{ keyword . }}{{ end }} {{- with typeHelper $option }} {{ . }}{{ end }}
    {{- with envName $option }}, {{ print "$" . | keyword }}{{ end }}
    {{- with $option.Default }} (default: {{ . }}){{ end }}
        {{- with $option.Description }}
            {{- $desc := $option.Description }}
{{ indent $desc 10 }}
{{- if isDeprecated $option }}{{ indent (printf "DEPRECATED: Use %s instead." (useInstead $option)) 10 }}{{ end }}
        {{- end -}}
    {{- end }}
{{- end }}
———
{{- if .Parent }}
Run `coder --help` for a list of global options.
{{- else }}
Report bugs and request features at https://github.com/coder/coder/issues/new
{{- end }}
