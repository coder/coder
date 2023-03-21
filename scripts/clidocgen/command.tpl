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
### --{{ $opt.Flag }}{{ with $opt.FlagShorthand}}, -{{ . }}{{end}}
{{" "}}
| | |
| --- | --- |
{{- with $opt.Env }}
| Environment | {{ (print "$" .) | wrapCode }} |
{{- end }}
{{- with $opt.Default }}
| Default | {{"    "}} {{- . | wrapCode }} |
{{ "" }}
{{ end }}
{{ "" }}
{{ $opt.Description | newLinesToBr }}
{{- end}}
