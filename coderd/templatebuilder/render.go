package templatebuilder

import (
	"bytes"
	"io/fs"
	"regexp"
	"text/template"

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

// agentResourcePattern matches `resource "coder_agent" "<name>"` in HCL.
var agentResourcePattern = regexp.MustCompile(`resource\s+"coder_agent"\s+"(\w+)"`)

// agentCountPattern detects whether a coder_agent block uses count or
// for_each, which means references to it require an index (e.g. [0]).
var agentCountPattern = regexp.MustCompile(
	`resource\s+"coder_agent"\s+"\w+"\s*\{[^}]*\b(?:count|for_each)\s*=`,
)

// ExtractAgentResourceName finds the coder_agent resource declaration in
// rendered HCL and returns the reference form to use in module templates.
// When the agent uses count or for_each, the returned name includes an
// index suffix (e.g. "dev[0]") so that module templates can reference it
// as coder_agent.<name>.id. Returns an error unless exactly one
// coder_agent resource is found; the builder only supports single-agent
// templates. The input is expected to be rendered output from our own
// curated base templates, not arbitrary user HCL.
func ExtractAgentResourceName(hcl []byte) (string, error) {
	matches := agentResourcePattern.FindAllSubmatch(hcl, -1)
	switch len(matches) {
	case 0:
		return "", xerrors.New("no coder_agent resource found in rendered template")
	case 1:
		name := string(matches[0][1])
		if agentCountPattern.Match(hcl) {
			name += "[0]"
		}
		return name, nil
	default:
		names := make([]string, 0, len(matches))
		for _, m := range matches {
			names = append(names, string(m[1]))
		}
		return "", xerrors.Errorf("expected exactly one coder_agent resource, found %d: %v",
			len(matches), names)
	}
}
