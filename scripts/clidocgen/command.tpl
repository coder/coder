<!-- DO NOT EDIT | GENERATED CONTENT -->
# {{ .Name }}

{{ .Cmd.Short }}

{{- if .Cmd.Runnable}}
## Usage
```console
{{.Cmd.UseLine}}
```
{{end}}

{{- if .Cmd.HasExample}}
## Examples
```console
{{.Cmd.Example}}
```
{{end}}

{{- range $index, $cmd := .Cmd.Commands }}
{{- if eq $index 0 }}
## Subcommands
| Name |   Purpose |
| ---- |   ----- |
{{- end }}
| {{ $cmd.Name | wrapCode }} | {{ $cmd.Short }} |
{{- end}}
{{ "" }}
{{- range $index, $flag := .Flags }}
{{- if eq $index 0 }}
## Local Flags
| Name |  Default | Usage |
| ---- |  ------- | ----- |
{{- end }}
| --{{ $flag.Name }}{{ if $flag.Shorthand}}, -{{ $flag.Shorthand }}{{end}} | {{ $flag.DefValue }} | {{ $flag.Usage | newLinesToBr | wrapCode }}|
{{- end}}
