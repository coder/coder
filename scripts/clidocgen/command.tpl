<!-- DO NOT EDIT | GENERATED CONTENT -->
# {{ fullName . }}

{{ with .Short }}
{{ . }}

{{ end }}

{{ with .Aliases }}
Aliases:
{{- range $index, $alias := . }}
* {{ $alias }}
{{- end }}
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
| [{{ $cmd.Name | wrapCode }}](./{{commandURI $cmd}}) | {{ $cmd.Short }} |
{{- end}}
{{ "" }}
{{- range $index, $opt := visibleOptions . }}
{{- if eq $index 0 }}
## Options
{{- end }}
### {{ with $opt.FlagShorthand}}-{{ . }}, {{end}}--{{ $opt.Flag }}
{{" "}}
{{ tableHeader }}
| Type | {{ typeHelper $opt | wrapCode }} |
{{- with $opt.Env }}
| Environment | {{ (print "$" .) | wrapCode }} |
{{- end }}
{{- with $opt.YAMLPath }}
| YAML | {{ . | wrapCode }} |
{{- end }}
{{- with $opt.Default }}
| Default | {{- . | wrapCode }} |
{{ "" }}
{{ end }}
{{ "" }}
{{ $opt.Description | newLinesToBr }}
{{- end}}
