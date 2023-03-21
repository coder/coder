<!-- DO NOT EDIT | GENERATED CONTENT -->
# {{ fullName . }}

{{ with .Short }}
{{ . }}

{{ end }}

{{- if .Use }}
## Usage
```console
{{ .FullUsage }}
```
{{end}}

{{- if .Long}}
## Description
```console
{{.Long}}
```
{{end}}

{{- range $index, $cmd := visibleSubcommands . }}
{{- if eq $index 0 }}
## Subcommands
| Name |   Purpose |
| ---- |   ----- |
{{- end }}
| [{{ $cmd.Name | wrapCode }}](./{{if atRoot $}}cli/{{end}}{{commandURI $cmd}}) | {{ $cmd.Short }} |
{{- end}}
{{ "" }}
{{- range $index, $opt := visibleOptions . }}
{{- if eq $index 0 }}
## Options
{{- end }}
### {{ with $opt.FlagShorthand}}-{{ . }}, {{end}}--{{ $opt.Flag }}
{{" "}}
{{ $printedHeader := false }}
{{- with $opt.Env }}
{{- if not $printedHeader }} {{ tableHeader }} {{ $printedHeader = true}} {{ end }}
| Environment | {{ (print "$" .) | wrapCode }} |
{{- end }}
{{- with $opt.Default }}
{{- if not $printedHeader }} {{ tableHeader }} {{ $printedHeader = true}} {{ end }}
| Default | {{"    "}} {{- . | wrapCode }} |
{{ "" }}
{{ end }}
{{ "" }}
{{ $opt.Description | newLinesToBr }}
{{- end}}
