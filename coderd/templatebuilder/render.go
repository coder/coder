package templatebuilder

import (
	"bytes"
	"io/fs"
	"text/template"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"golang.org/x/xerrors"
)

// ImageOption represents a container image choice for base template parameters.
type ImageOption struct {
	Name  string
	Value string
}

// DefaultRegistryBase is the module registry host used in rendered module source
// paths when a ComposeRequest does not carry the deployment's configured
// registry (CODER_TEMPLATE_BUILDER_REGISTRY_URL). It mirrors the default of the
// codersdk template-builder registry option.
const DefaultRegistryBase = "registry.coder.com"

// BaseRenderContext is the data passed to base template .tf.tmpl files.
type BaseRenderContext struct {
	ContainerImage string
	ImageOptions   []ImageOption
	// RegistryBase is the module registry host used in rendered module source
	// paths, mirroring ModuleRenderContext.RegistryBase so a base-embedded
	// module honors CODER_TEMPLATE_BUILDER_REGISTRY_URL like a wizard module.
	RegistryBase string
	Variables    map[string]string
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

// parseRenderedHCL parses rendered Terraform into an hclsyntax body. The input
// is expected to be rendered output from our own curated base templates, so a
// parse error indicates a bug in a template rather than untrusted user input.
func parseRenderedHCL(rendered []byte) (*hclsyntax.Body, error) {
	file, diags := hclsyntax.ParseConfig(rendered, "rendered.tf", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, xerrors.Errorf("parse rendered template HCL: %s", diags.Error())
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil, xerrors.New("unexpected HCL body type")
	}
	return body, nil
}

// ExtractedAgent describes a coder_agent resource found in rendered HCL.
type ExtractedAgent struct {
	// Name is the Terraform resource name (the second block label).
	Name string
	// Reference is the form to use in module templates: Name, suffixed with
	// [0] when the agent uses count or for_each and therefore requires an
	// index (e.g. coder_agent.dev[0].id).
	Reference string
}

// ExtractAgentResourceNames returns every coder_agent resource declared in
// rendered HCL, in declaration order. Unlike the singular helper it does not
// error on multiple agents: enumerating them all lets a base declare more than
// one agent. Reading count/for_each from each block body is robust to nested
// blocks (env {}, metadata {}). The input is expected to be rendered output
// from our own curated base templates, not arbitrary user HCL.
func ExtractAgentResourceNames(rendered []byte) ([]ExtractedAgent, error) {
	body, err := parseRenderedHCL(rendered)
	if err != nil {
		return nil, err
	}
	var agents []ExtractedAgent
	for _, block := range body.Blocks {
		if block.Type != "resource" || len(block.Labels) < 2 || block.Labels[0] != "coder_agent" {
			continue
		}
		ref := block.Labels[1]
		if _, ok := block.Body.Attributes["count"]; ok {
			ref += "[0]"
		} else if _, ok := block.Body.Attributes["for_each"]; ok {
			ref += "[0]"
		}
		agents = append(agents, ExtractedAgent{Name: block.Labels[1], Reference: ref})
	}
	return agents, nil
}

// ExtractAgentResourceName finds the coder_agent resource declaration in
// rendered HCL and returns the reference form to use in module templates.
// When the agent uses count or for_each, the returned name includes an
// index suffix (e.g. "dev[0]") so that module templates can reference it
// as coder_agent.<name>.id. Returns an error unless exactly one
// coder_agent resource is found; callers that support multiple agents
// should use ExtractAgentResourceNames instead. The input is expected to be
// rendered output from our own curated base templates, not arbitrary user HCL.
func ExtractAgentResourceName(rendered []byte) (string, error) {
	agents, err := ExtractAgentResourceNames(rendered)
	if err != nil {
		return "", err
	}
	switch len(agents) {
	case 0:
		return "", xerrors.New("no coder_agent resource found in rendered template")
	case 1:
		return agents[0].Reference, nil
	default:
		names := make([]string, 0, len(agents))
		for _, a := range agents {
			names = append(names, a.Name)
		}
		return "", xerrors.Errorf("expected exactly one coder_agent resource, found %d: %v",
			len(agents), names)
	}
}

// ExtractModuleNames returns the labels of every module block declared in
// rendered HCL, in declaration order. Because it walks parsed blocks,
// commented-out references and module strings inside heredocs are naturally
// ignored. The input is expected to be rendered output from our own curated
// base templates, not arbitrary user HCL; unparsable input yields no names.
func ExtractModuleNames(rendered []byte) []string {
	body, err := parseRenderedHCL(rendered)
	if err != nil {
		return nil
	}
	var names []string
	for _, block := range body.Blocks {
		if block.Type == "module" && len(block.Labels) >= 1 {
			names = append(names, block.Labels[0])
		}
	}
	return names
}
