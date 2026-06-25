package templatebuilder

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"maps"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"golang.org/x/xerrors"
)

// maxStringValueLen is the maximum byte length for a string variable value.
const maxStringValueLen = 4096

// numberPattern matches valid HCL number literals (integers and decimals).
var numberPattern = regexp.MustCompile(`^-?[0-9]+(\.[0-9]+)?$`)

// ComposeRequest describes which base template and modules to render.
type ComposeRequest struct {
	BaseTemplateID string
	// BaseVariableValues maps base template variable names to their
	// user-supplied values.
	BaseVariableValues map[string]string
	// RegistryURL is the module registry base URL from the deployment
	// config (CODER_TEMPLATE_BUILDER_REGISTRY_URL).
	RegistryURL string
	Modules     []ComposeModule
}

// ComposeModule identifies a module to include and the variable values
// to render into its module block.
type ComposeModule struct {
	ID string
	// Variables maps variable names to HCL literal values for
	// non-sensitive, non-computed variables.
	Variables map[string]string
}

// ComposeResult holds the rendered Terraform files ready for bundling.
type ComposeResult struct {
	// MainTF is the rendered base template.
	MainTF []byte
	// ModulesTF is the concatenated rendered module blocks. Empty when
	// no modules are selected.
	ModulesTF []byte
	// Readme is the full README.md content from the base template.
	// Empty when the base has no README.
	Readme []byte
}

// Compose renders a base template and selected modules into Terraform
// source files. It extracts the coder_agent resource name from the
// rendered base HCL and wires it into each module block.
func Compose(req ComposeRequest) (*ComposeResult, error) {
	mainTF, err := renderBase(req.BaseTemplateID, req.BaseVariableValues)
	if err != nil {
		return nil, err
	}

	if len(req.Modules) == 0 {
		return &ComposeResult{
			MainTF: formatHCL(mainTF),
			Readme: []byte(BaseReadme(req.BaseTemplateID)),
		}, nil
	}

	agentName, err := ExtractAgentResourceName(mainTF)
	if err != nil {
		return nil, xerrors.Errorf("extract agent name: %w", err)
	}

	catalog, err := loadCatalogMap()
	if err != nil {
		return nil, err
	}

	baseOS := BaseTemplateOS(req.BaseTemplateID)
	if err := validateModules(req.Modules, catalog, baseOS); err != nil {
		return nil, err
	}

	modulesTF, err := renderModules(req.Modules, catalog, req.RegistryURL, agentName)
	if err != nil {
		return nil, err
	}

	result := &ComposeResult{
		MainTF:    formatHCL(mainTF),
		ModulesTF: formatHCL(modulesTF),
		Readme:    []byte(BaseReadme(req.BaseTemplateID)),
	}
	return result, nil
}

// formatHCL applies canonical HCL formatting to src. If src is not valid
// HCL the input is returned unchanged.
func formatHCL(src []byte) []byte {
	if len(src) == 0 {
		return src
	}
	return hclwrite.Format(src)
}

// renderBase renders the base template for the given example ID,
// merging any user-supplied variable values into the render context.
func renderBase(baseTemplateID string, baseVars map[string]string) ([]byte, error) {
	renderCtx := DefaultBaseRenderContext(baseTemplateID)
	if renderCtx.Variables == nil {
		renderCtx.Variables = make(map[string]string)
	}

	vars, err := mergeBaseVariables(baseTemplateID, baseVars)
	if err != nil {
		return nil, xerrors.Errorf("base %q: %w", baseTemplateID, err)
	}
	maps.Copy(renderCtx.Variables, vars)

	mainTF, err := RenderBaseTemplate(baseTemplateID, "main.tf.tmpl", renderCtx)
	if err != nil {
		return nil, xerrors.Errorf("render base template: %w", err)
	}
	return mainTF, nil
}

// mergeBaseVariables builds the final Variables map for a base template.
// It starts with manifest defaults, overlays caller-supplied values,
// validates types, and converts to HCL literals.
func mergeBaseVariables(baseTemplateID string, callerVars map[string]string) (map[string]string, error) {
	allVars := BaseVariables(baseTemplateID)
	if len(allVars) == 0 && len(callerVars) == 0 {
		return make(map[string]string), nil
	}

	allowedVars := make(map[string]ModuleVariable, len(allVars))
	for _, v := range allVars {
		if v.Computed || v.Sensitive {
			continue
		}
		allowedVars[v.Name] = v
	}

	// Validate caller-supplied keys and values.
	for k, val := range callerVars {
		v, ok := allowedVars[k]
		if !ok {
			return nil, xerrors.Errorf("unknown variable %q", k)
		}
		if err := validateVariableValue(v, val); err != nil {
			return nil, xerrors.Errorf("variable %q: %w", k, err)
		}
	}

	// Build merged map from manifest defaults.
	merged := make(map[string]string, len(allVars))
	for _, v := range allVars {
		if v.Computed || v.Sensitive {
			continue
		}
		if len(v.Default) > 0 && isSimpleJSONValue(v.Default) {
			merged[v.Name] = string(v.Default)
		}
	}

	// Overlay validated caller values, converting to HCL literals.
	for k, val := range callerVars {
		merged[k] = toHCLLiteral(allowedVars[k], val)
	}

	// Ensure all required variables without defaults have a value.
	for _, v := range allVars {
		if v.Computed || v.Sensitive {
			continue
		}
		if v.Required && merged[v.Name] == "" {
			return nil, xerrors.Errorf("variable %q is required", v.Name)
		}
	}

	return merged, nil
}

// loadCatalogMap loads the module catalog and returns it as a map keyed
// by module ID.
func loadCatalogMap() (map[string]ModuleManifest, error) {
	modules, err := LoadModules()
	if err != nil {
		return nil, xerrors.Errorf("load module catalog: %w", err)
	}
	catalog := make(map[string]ModuleManifest, len(modules))
	for _, m := range modules {
		catalog[m.ID] = m
	}
	return catalog, nil
}

// validateModules checks that all requested modules exist, are
// OS-compatible, have no duplicates, and have no conflicts.
func validateModules(requested []ComposeModule, catalog map[string]ModuleManifest, baseOS BaseOS) error {
	seen := make(map[string]bool, len(requested))
	for _, cm := range requested {
		if seen[cm.ID] {
			return xerrors.Errorf("duplicate module %q", cm.ID)
		}
		seen[cm.ID] = true

		manifest, ok := catalog[cm.ID]
		if !ok {
			return xerrors.Errorf("unknown module %q", cm.ID)
		}
		if !manifest.CompatibleWithOS(string(baseOS)) {
			return xerrors.Errorf("module %q is not compatible with OS %q", cm.ID, baseOS)
		}
	}

	// Check conflicts bidirectionally so that order does not matter.
	for _, cm := range requested {
		manifest := catalog[cm.ID]
		for _, conflict := range manifest.ConflictsWith {
			if seen[conflict] {
				return xerrors.Errorf("module %q conflicts with %q", cm.ID, conflict)
			}
		}
	}

	return nil
}

// renderModules renders each module template and concatenates the
// results with newline separators.
func renderModules(
	requested []ComposeModule,
	catalog map[string]ModuleManifest,
	registryURL, agentName string,
) ([]byte, error) {
	var buf bytes.Buffer
	for _, cm := range requested {
		manifest := catalog[cm.ID]

		modFS, err := ModuleTemplateFS(cm.ID)
		if err != nil {
			return nil, xerrors.Errorf("module template FS for %q: %w", cm.ID, err)
		}

		vars, err := mergeModuleVariables(manifest, cm.Variables)
		if err != nil {
			return nil, xerrors.Errorf("module %q: %w", cm.ID, err)
		}
		modCtx := ModuleRenderContext{
			RegistryBase:      registryURL,
			PinnedVersion:     manifest.PinnedVersion,
			AgentResourceName: agentName,
			Variables:         vars,
		}

		rendered, err := RenderModuleTemplate(modFS, cm.ID+".tf.tmpl", modCtx)
		if err != nil {
			return nil, xerrors.Errorf("render module %q: %w", cm.ID, err)
		}

		if buf.Len() > 0 {
			_ = buf.WriteByte('\n')
		}
		_, _ = buf.Write(rendered)
	}
	return buf.Bytes(), nil
}

// mergeModuleVariables builds the final Variables map for a module template.
// It starts with manifest defaults for all non-computed, non-sensitive
// variables, then overlays caller-supplied values. Caller-supplied keys
// are validated against the manifest and values are checked for type
// correctness before being accepted.
func mergeModuleVariables(manifest ModuleManifest, callerVars map[string]string) (map[string]string, error) {
	// Build lookup structures for the manifest variables.
	allowedVars := make(map[string]ModuleVariable, len(manifest.Variables))
	for _, v := range manifest.Variables {
		if v.Computed || v.Sensitive {
			continue
		}
		allowedVars[v.Name] = v
	}

	// Validate caller-supplied keys and values before merging.
	for k, val := range callerVars {
		v, ok := allowedVars[k]
		if !ok {
			return nil, xerrors.Errorf("unknown variable %q", k)
		}
		if err := validateVariableValue(v, val); err != nil {
			return nil, xerrors.Errorf("variable %q: %w", k, err)
		}
	}

	// Build merged map from manifest defaults.
	merged := make(map[string]string, len(manifest.Variables))
	for _, v := range manifest.Variables {
		if v.Computed || v.Sensitive {
			continue
		}
		if len(v.Default) > 0 && isSimpleJSONValue(v.Default) {
			// json.RawMessage values for simple types (e.g. `""`,
			// `false`, `13337`) are valid HCL literals.
			merged[v.Name] = string(v.Default)
		} else if !v.Required {
			// Non-required variables without an explicit default use
			// null, which tells Terraform to apply the module's own
			// default.
			merged[v.Name] = "null"
		}
		// Required variables without defaults are left out so that
		// missingkey=error surfaces the omission at render time.
	}

	// Overlay validated caller values, converting to HCL literals.
	for k, val := range callerVars {
		merged[k] = toHCLLiteral(allowedVars[k], val)
	}

	// Ensure all required variables without defaults have a value.
	for _, v := range manifest.Variables {
		if v.Computed || v.Sensitive {
			continue
		}
		if v.Required && merged[v.Name] == "" {
			return nil, xerrors.Errorf("variable %q is required", v.Name)
		}
	}

	return merged, nil
}

// validateVariableValue checks that the caller-supplied value is valid for
// the variable's declared type. String values are plain text (not
// pre-quoted); quoting for HCL happens later in toHCLLiteral.
// The literal "null" is accepted for any type.
func validateVariableValue(v ModuleVariable, value string) error {
	if value == "null" {
		return nil
	}
	switch v.Type {
	case "string":
		return validateStringValue(value)
	case "number":
		return validateNumberValue(value)
	case "bool":
		return validateBoolValue(value)
	default:
		return xerrors.Errorf("unsupported variable type %q", v.Type)
	}
}

// toHCLLiteral converts a validated caller value into an HCL literal.
// The literal "null" is passed through for any type. Strings are wrapped
// in quotes with interior characters escaped; bools and numbers are
// already valid HCL literals.
func toHCLLiteral(v ModuleVariable, value string) string {
	if value == "null" {
		return value
	}
	if v.Type == "string" {
		return hclQuote(value)
	}
	return value
}

// validateStringValue checks that a raw (unquoted) string value is safe
// to embed in an HCL quoted string. It rejects HCL interpolation/directive
// markers and values that exceed the maximum length.
func validateStringValue(value string) error {
	if len(value) > maxStringValueLen {
		return xerrors.Errorf("value exceeds maximum length of %d bytes", maxStringValueLen)
	}
	if strings.Contains(value, "${") || strings.Contains(value, "%{") {
		return xerrors.New("must not contain HCL interpolation or directive sequences")
	}
	return nil
}

// hclQuote wraps a raw string in HCL double-quotes, escaping backslashes,
// double-quotes, and newlines so the result is a valid HCL string literal.
func hclQuote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	_, _ = b.WriteRune('"')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			_, _ = b.WriteString("\\\\")
		case '"':
			_, _ = b.WriteString("\\\"")
		case '\n':
			_, _ = b.WriteString("\\n")
		case '\r':
			_, _ = b.WriteString("\\r")
		default:
			_ = b.WriteByte(s[i])
		}
	}
	_, _ = b.WriteRune('"')
	return b.String()
}

// validateNumberValue checks that value is a valid HCL number literal.
func validateNumberValue(value string) error {
	if !numberPattern.MatchString(value) {
		return xerrors.Errorf("invalid number value %q, must be a numeric literal (e.g. 42, 3.14)", value)
	}
	return nil
}

// validateBoolValue checks that value is exactly "true" or "false".
func validateBoolValue(value string) error {
	if value != "true" && value != "false" {
		return xerrors.Errorf("invalid bool value %q, must be true or false", value)
	}
	return nil
}

// isSimpleJSONValue returns true if raw is a valid JSON string, number,
// bool, or null. Arrays and objects are rejected; the template builder
// only supports simple variable types.
func isSimpleJSONValue(raw json.RawMessage) bool {
	var v interface{}
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	switch v.(type) {
	case string, float64, bool, nil:
		return true
	default:
		return false
	}
}

// BundleTar packages the compose result into a tar archive suitable for
// the Coder file store.
func BundleTar(result *ComposeResult) ([]byte, error) {
	if result == nil {
		return nil, xerrors.New("nil ComposeResult")
	}

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if err := writeTarFile(tw, "main.tf", result.MainTF); err != nil {
		return nil, xerrors.Errorf("write main.tf to tar: %w", err)
	}

	if len(result.ModulesTF) > 0 {
		if err := writeTarFile(tw, "modules.tf", result.ModulesTF); err != nil {
			return nil, xerrors.Errorf("write modules.tf to tar: %w", err)
		}
	}

	if len(result.Readme) > 0 {
		if err := writeTarFile(tw, "README.md", result.Readme); err != nil {
			return nil, xerrors.Errorf("write README.md to tar: %w", err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, xerrors.Errorf("close tar writer: %w", err)
	}

	return buf.Bytes(), nil
}

// writeTarFile adds a single file entry to a tar writer. It uses a zero
// timestamp for reproducible archives.
func writeTarFile(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Mode:    0o644,
		Size:    int64(len(data)),
		ModTime: time.Unix(0, 0),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}
