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

{{- range $index, $flag := .Flags }}
{{- if eq $index 0 }}
## Local Flags
| Name |  Default | Usage |
| ---- |  ------- | ----- |
{{- end }}
| --{{ $flag.Name }}{{ if $flag.Shorthand}}, -{{ $flag.Shorthand }}{{end}} | {{ $flag.DefValue }} | {{ $flag.Usage | newLinesToBr | wrapCode }}|
{{- end}}
