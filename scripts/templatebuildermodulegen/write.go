package main

import (
	"encoding/json"
	"os"
	"text/template"
)

func writeModuleJSON(path string, m ModuleManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o600)
}

var tfTmplTemplate = template.Must(template.New("tf.tmpl").Funcs(template.FuncMap{
	"hclValue": func(v ModuleVariable) string {
		if v.Sensitive {
			return "var." + v.Name
		}
		return "{{ .Variables." + v.Name + " }}"
	},
}).Parse(`{{- range .SensitiveVars }}
variable "{{ .Name }}" {
  description = "{{ .Description }}"
  type        = {{ .Type }}
  sensitive   = true
  default     = ""
}
{{ end -}}
module "{{ .ID }}" {
  count    = data.coder_workspace.me.start_count
  source   = "{{"{{"}} .RegistryBase {{"}}"}}/{{ .Namespace }}/{{ .ID }}/coder"
  version  = "{{"{{"}} .PinnedVersion {{"}}"}}"
  agent_id = coder_agent.{{"{{"}} .AgentResourceName {{"}}"}}.id
{{- range .NonComputedVars }}
{{- if .Sensitive }}
  {{ .Name }} = var.{{ .Name }}
{{- else }}
  {{ .Name }} = {{"{{"}} .Variables.{{ .Name }} {{"}}"}}
{{- end }}
{{- end }}
}
`))

type tfTmplData struct {
	ID              string
	Namespace       string
	SensitiveVars   []ModuleVariable
	NonComputedVars []ModuleVariable
}

func writeTFTmpl(path string, m ModuleManifest) error {
	var sensitiveVars []ModuleVariable
	var nonComputedVars []ModuleVariable
	for _, v := range m.Variables {
		if v.Computed {
			continue
		}
		nonComputedVars = append(nonComputedVars, v)
		if v.Sensitive {
			sensitiveVars = append(sensitiveVars, v)
		}
	}

	data := tfTmplData{
		ID:              m.ID,
		Namespace:       m.Namespace,
		SensitiveVars:   sensitiveVars,
		NonComputedVars: nonComputedVars,
	}

	f, err := os.Create(path)
	if err != nil {
		return err
	}
	err = tfTmplTemplate.Execute(f, data)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	return err
}
