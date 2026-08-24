package templatebuilder

import (
	"bytes"
	"io/fs"
	"text/template"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
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

// ExtractAgentResourceName finds the coder_agent resource declaration in
// rendered HCL and returns the reference form to use in module templates.
// When the agent uses count or for_each, the returned name includes an
// index suffix (e.g. "dev[0]") so that module templates can reference it
// as coder_agent.<name>.id. Returns an error unless exactly one
// coder_agent resource is found; the builder only supports single-agent
// templates. Parsing with hclsyntax means count/for_each is detected
// wherever it appears in the block, not only before the first closing brace.
// The input is expected to be rendered output from our own curated base
// templates, not arbitrary user HCL.
func ExtractAgentResourceName(hclSrc []byte) (string, error) {
	body := parseHCLBody(hclSrc)
	if body == nil {
		return "", xerrors.New("no coder_agent resource found in rendered template")
	}

	var names []string
	counted := false
	for _, block := range body.Blocks {
		if block.Type != "resource" || len(block.Labels) != 2 || block.Labels[0] != "coder_agent" {
			continue
		}
		names = append(names, block.Labels[1])
		if _, ok := block.Body.Attributes["count"]; ok {
			counted = true
		}
		if _, ok := block.Body.Attributes["for_each"]; ok {
			counted = true
		}
	}

	switch len(names) {
	case 0:
		return "", xerrors.New("no coder_agent resource found in rendered template")
	case 1:
		name := names[0]
		if counted {
			name += "[0]"
		}
		return name, nil
	default:
		return "", xerrors.Errorf("expected exactly one coder_agent resource, found %d: %v",
			len(names), names)
	}
}

// parseHCLBody parses rendered HCL from one of our curated base templates and
// returns its top-level body. The input is expected to be valid HCL produced by
// our own templates; on a parse error it returns nil so callers fail safe (they
// yield an empty result, which trips the NotEmpty/match assertions in the tests
// that consume them rather than passing silently).
func parseHCLBody(hclSrc []byte) *hclsyntax.Body {
	file, diags := hclsyntax.ParseConfig(hclSrc, "rendered.tf", hcl.InitialPos)
	if diags.HasErrors() || file == nil {
		return nil
	}
	body, ok := file.Body.(*hclsyntax.Body)
	if !ok {
		return nil
	}
	return body
}

// ExtractModuleNames returns the labels of every top-level `module` block in
// rendered HCL, in declaration order. Parsing with hclsyntax means commented-out
// blocks and module-like text inside strings or heredocs are ignored. The input
// is expected to be rendered output from our own curated base templates, not
// arbitrary user HCL.
func ExtractModuleNames(hclSrc []byte) []string {
	body := parseHCLBody(hclSrc)
	if body == nil {
		return nil
	}
	var names []string
	for _, block := range body.Blocks {
		if block.Type == "module" && len(block.Labels) > 0 {
			names = append(names, block.Labels[0])
		}
	}
	return names
}

// ExtractParameterOptionValues returns the value of every `option` block inside
// the `data "coder_parameter" "<paramName>"` block in rendered HCL, in
// declaration order. Returns nil if the parameter is absent. The input is
// expected to be rendered output from our own curated base templates.
func ExtractParameterOptionValues(hclSrc []byte, paramName string) []string {
	body := parseHCLBody(hclSrc)
	if body == nil {
		return nil
	}
	var values []string
	for _, block := range body.Blocks {
		if block.Type != "data" || len(block.Labels) != 2 ||
			block.Labels[0] != "coder_parameter" || block.Labels[1] != paramName {
			continue
		}
		for _, option := range block.Body.Blocks {
			if option.Type != "option" {
				continue
			}
			if attr, ok := option.Body.Attributes["value"]; ok {
				if s, ok := staticString(attr.Expr); ok {
					values = append(values, s)
				}
			}
		}
	}
	return values
}

// ExtractPresetParameterValues returns, for every `data "coder_workspace_preset"`
// block in rendered HCL, the string list assigned to paramName in its
// `parameters` object (e.g. "languages"). A value written as jsonencode([...])
// is unwrapped. Presets that do not set the key are skipped. The input is
// expected to be rendered output from our own curated base templates.
func ExtractPresetParameterValues(hclSrc []byte, paramName string) [][]string {
	body := parseHCLBody(hclSrc)
	if body == nil {
		return nil
	}
	var out [][]string
	for _, block := range body.Blocks {
		if block.Type != "data" || len(block.Labels) < 1 ||
			block.Labels[0] != "coder_workspace_preset" {
			continue
		}
		params, ok := block.Body.Attributes["parameters"]
		if !ok {
			continue
		}
		pairs, diags := hcl.ExprMap(params.Expr)
		if diags.HasErrors() {
			continue
		}
		for _, pair := range pairs {
			if exprKeyString(pair.Key) != paramName {
				continue
			}
			if list, ok := staticStringList(pair.Value); ok {
				out = append(out, list)
			}
		}
	}
	return out
}

// ExtractPresetNames returns the labels of every top-level
// `data "coder_workspace_preset"` block in rendered HCL, in declaration order.
// The input is expected to be rendered output from our own curated base
// templates.
func ExtractPresetNames(hclSrc []byte) []string {
	body := parseHCLBody(hclSrc)
	if body == nil {
		return nil
	}
	var names []string
	for _, block := range body.Blocks {
		if block.Type == "data" && len(block.Labels) == 2 &&
			block.Labels[0] == "coder_workspace_preset" {
			names = append(names, block.Labels[1])
		}
	}
	return names
}

// staticString evaluates an expression expected to be a constant string (no
// variables or functions) and reports whether it produced one.
func staticString(expr hcl.Expression) (string, bool) {
	val, diags := expr.Value(nil)
	if diags.HasErrors() || val.IsNull() || val.Type() != cty.String {
		return "", false
	}
	return val.AsString(), true
}

// staticStringList evaluates an expression expected to be a constant list of
// strings, unwrapping a single jsonencode(...) call, and reports whether it
// produced one.
func staticStringList(expr hcl.Expression) ([]string, bool) {
	if call, ok := expr.(*hclsyntax.FunctionCallExpr); ok && call.Name == "jsonencode" && len(call.Args) == 1 {
		expr = call.Args[0]
	}
	val, diags := expr.Value(nil)
	if diags.HasErrors() || val.IsNull() || !val.CanIterateElements() {
		return nil, false
	}
	out := make([]string, 0, val.LengthInt())
	for it := val.ElementIterator(); it.Next(); {
		_, ev := it.Element()
		if ev.Type() != cty.String {
			return nil, false
		}
		out = append(out, ev.AsString())
	}
	return out, true
}

// exprKeyString returns the string key of an object-construction key
// expression, handling bare identifier keys (e.g. `languages = ...`).
func exprKeyString(key hcl.Expression) string {
	if kw := hcl.ExprAsKeyword(key); kw != "" {
		return kw
	}
	if val, diags := key.Value(nil); !diags.HasErrors() && val.Type() == cty.String {
		return val.AsString()
	}
	return ""
}
