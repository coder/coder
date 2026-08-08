package templatebuilder

import (
	"bytes"
	"io/fs"
	"text/template"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"golang.org/x/xerrors"
)

// ImageOption represents a container image choice for base template parameters.
type ImageOption struct {
	Name  string
	Value string
}

// BaseRenderContext is the data passed to base template .tf.tmpl files.
type BaseRenderContext struct {
	ContainerImage string
	ImageOptions   []ImageOption
	Variables      map[string]string
}

// ModuleRenderContext is the data passed to module .tf.tmpl files.
type ModuleRenderContext struct {
	// RegistryBase is the module registry URL from the deployment config
	// (CODER_TEMPLATE_BUILDER_REGISTRY_URL).
	RegistryBase string
	// PinnedVersion is the module version from the catalog manifest.
	PinnedVersion string
	// AgentResourceName is the Terraform resource name of the coder_agent
	// declared in the base template (e.g. "main" or "dev").
	AgentResourceName string
	// Variables maps variable names to their HCL expressions.
	Variables map[string]string
}

// RenderBaseTemplate executes a pre-parsed .tf.tmpl template for the given
// base, applying the provided render context. Templates are parsed once at
// first access via sync.OnceValues, so parse errors surface early instead
// of at render time.
func RenderBaseTemplate(exampleID, templatePath string, renderCtx BaseRenderContext) ([]byte, error) {
	if renderCtx.Variables == nil {
		renderCtx.Variables = make(map[string]string)
	}

	bases, err := loadBases()
	if err != nil {
		return nil, xerrors.Errorf("load base catalog: %w", err)
	}

	base, ok := bases[exampleID]
	if !ok {
		return nil, xerrors.Errorf("unknown base template %q", exampleID)
	}

	tmpl, ok := base.Templates[templatePath]
	if !ok {
		return nil, xerrors.Errorf("template %s not found in base %q", templatePath, exampleID)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, renderCtx); err != nil {
		return nil, xerrors.Errorf("execute template %s: %w", templatePath, err)
	}

	return buf.Bytes(), nil
}

// RenderModuleTemplate parses and executes a module .tf.tmpl file from
// the given filesystem, applying the provided render context.
func RenderModuleTemplate(fsys fs.FS, templatePath string, renderCtx ModuleRenderContext) ([]byte, error) {
	if renderCtx.Variables == nil {
		renderCtx.Variables = make(map[string]string)
	}
	return renderTemplate(fsys, templatePath, renderCtx)
}

// renderTemplate is the shared implementation for module template rendering.
// It sets missingkey=error so that references to undefined variable keys fail
// loudly instead of producing "<no value>".
func renderTemplate(fsys fs.FS, templatePath string, data any) ([]byte, error) {
	raw, err := fs.ReadFile(fsys, templatePath)
	if err != nil {
		return nil, xerrors.Errorf("read template %s: %w", templatePath, err)
	}

	tmpl, err := template.New(templatePath).Option("missingkey=error").Parse(string(raw))
	if err != nil {
		return nil, xerrors.Errorf("parse template %s: %w", templatePath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, xerrors.Errorf("execute template %s: %w", templatePath, err)
	}

	return buf.Bytes(), nil
}

// ExtractedAgent describes a coder_agent resource found in rendered HCL.
type ExtractedAgent struct {
	// Name is the bare Terraform resource name, e.g. "main" or "dev".
	Name string
	// Reference is the form used to reference the agent in HCL. When the
	// agent uses count or for_each it includes an index suffix (e.g.
	// "dev[0]") so that module templates can reference it as
	// coder_agent.<Reference>.id.
	Reference string
}

// ExtractAgentResourceNames finds every coder_agent resource declaration
// in rendered HCL, in document order. When an agent uses count or
// for_each its Reference includes an index suffix. Returns an error only
// when the HCL cannot be parsed or declares no coder_agent resource. The
// input is expected to be rendered output from our own curated base
// templates, not arbitrary user HCL.
func ExtractAgentResourceNames(hclSrc []byte) ([]ExtractedAgent, error) {
	file, diags := hclwrite.ParseConfig(hclSrc, "main.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, xerrors.Errorf("parse rendered template HCL: %s", diags.Error())
	}

	var agents []ExtractedAgent
	for _, block := range file.Body().Blocks() {
		if block.Type() != "resource" {
			continue
		}
		labels := block.Labels()
		if len(labels) != 2 || labels[0] != "coder_agent" {
			continue
		}
		name := labels[1]
		ref := name
		body := block.Body()
		if body.GetAttribute("count") != nil || body.GetAttribute("for_each") != nil {
			ref = name + "[0]"
		}
		agents = append(agents, ExtractedAgent{Name: name, Reference: ref})
	}

	if len(agents) == 0 {
		return nil, xerrors.New("no coder_agent resource found in rendered template")
	}
	return agents, nil
}

// ExtractAgentResourceName returns the reference form of the single
// coder_agent resource in rendered HCL. It errors unless exactly one
// coder_agent is found. Prefer ExtractAgentResourceNames for templates
// that may declare multiple agents.
func ExtractAgentResourceName(hclSrc []byte) (string, error) {
	agents, err := ExtractAgentResourceNames(hclSrc)
	if err != nil {
		return "", err
	}
	if len(agents) != 1 {
		names := make([]string, 0, len(agents))
		for _, a := range agents {
			names = append(names, a.Name)
		}
		return "", xerrors.Errorf("expected exactly one coder_agent resource, found %d: %v",
			len(agents), names)
	}
	return agents[0].Reference, nil
}
