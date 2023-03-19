<!-- DO NOT EDIT | GENERATED CONTENT -->
# {{ .Name }}
{{ if .Cmd.Long }}
{{ .Cmd.Long }}
{{ else }}
{{ .Cmd.Short }}
{{ end }}

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

{{- range $index, $cmd := .VisibleSubcommands }}
{{- if eq $index 0 }}
## Subcommands
| Name |   Purpose |
| ---- |   ----- |
{{- end }}
| [{{ $cmd.Name | wrapCode }}](./{{if $.AtRoot}}cli/{{end}}{{commandURI $cmd}}) | {{ $cmd.Short }} |
{{- end}}
{{ "" }}
{{- range $index, $flag := .Flags }}
{{- if eq $index 0 }}
## Flags
{{- end }}
### --{{ $flag.Name }}{{ if $flag.Shorthand}}, -{{ $flag.Shorthand }}{{end}}
{{ $flag.Usage | stripEnv | newLinesToBr }}
<br/>
| | |
| --- | --- |
{{- with $flag.Usage | parseEnv }}
| Consumes | {{ . | wrapCode }} |
{{- end }}
{{- with $flag.DefValue }}
| Default | {{"    "}} {{- . | wrapCode }} |
{{ "" }}
{{ end }}
{{ "" }}
{{- end}}
