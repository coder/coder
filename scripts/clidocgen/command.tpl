<!-- DO NOT EDIT | GENERATED CONTENT -->
# {{ .Name }}

{{ with .Short }} 
{{ . }}

{{ end }}

{{- if .Runnable}}
## Usage
```console
{{.Usage}}
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
| [{{ $cmd.Name | wrapCode }}](./{{if $.AtRoot}}cli/{{end}}{{commandURI $cmd}}) | {{ $cmd.Short }} |
{{- end}}
{{ "" }}
{{- range $index, $opt := .Options }}
{{- if eq $index 0 }}
## Options
{{- end }}
### --{{ $opt.Flag }}{{ with $opt.FlagShorthand}}, -{{ . }}{{end}}
{{ $opt.Usage | newLinesToBr }}
<br/>
| | |
| --- | --- |
{{- with $flag.Description }}
| Consumes | {{ . | wrapCode }} |
{{- end }}
{{- with $flag.Default }}
| Default | {{"    "}} {{- . | wrapCode }} |
{{ "" }}
{{ end }}
{{ "" }}
{{- end}}
