package templatebuilder

import (
	"archive/tar"
	"bytes"
	"encoding/json"
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
}

// Compose renders a base template and selected modules into Terraform
// source files. It extracts the coder_agent resource name from the
// rendered base HCL and wires it into each module block.
func Compose(req ComposeRequest) (*ComposeResult, error) {
	mainTF, err := renderBase(req.BaseTemplateID)
	if err != nil {
		return nil, err
	}

	if len(req.Modules) == 0 {
		return &ComposeResult{MainTF: formatHCL(mainTF)}, nil
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

	return &ComposeResult{
		MainTF:    formatHCL(mainTF),
		ModulesTF: formatHCL(modulesTF),
	}, nil
}

// formatHCL applies canonical HCL formatting to src. If src is not valid
// HCL the input is returned unchanged.
func formatHCL(src []byte) []byte {
	if len(src) == 0 {
		return src
	}
	return hclwrite.Format(src)
}

// renderBase renders the base template for the given example ID.
func renderBase(baseTemplateID string) ([]byte, error) {
	renderCtx := DefaultBaseRenderContext(baseTemplateID)
	mainTF, err := RenderBaseTemplate(baseTemplateID, "main.tf.tmpl", renderCtx)
	if err != nil {
		return nil, xerrors.Errorf("render base template: %w", err)
	}
	return mainTF, nil
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

	// Overlay validated caller values.
	for k, val := range callerVars {
		merged[k] = val
	}
	return merged, nil
}

// validateVariableValue checks that value is a valid HCL literal for the
// variable's declared type. The literal "null" is accepted for any type.
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

// validateStringValue checks that value is a valid quoted HCL string literal.
// It must start and end with '"', contain no unescaped newlines or quotes,
// and must not contain HCL interpolation/directive markers.
func validateStringValue(value string) error {
	if len(value) > maxStringValueLen {
		return xerrors.Errorf("value exceeds maximum length of %d bytes", maxStringValueLen)
	}
	if len(value) < 2 || value[0] != '"' || value[len(value)-1] != '"' {
		return xerrors.New("must be a quoted string (e.g. \"value\")")
	}

	inner := value[1 : len(value)-1]

	if strings.Contains(inner, "${") || strings.Contains(inner, "%{") {
		return xerrors.New("must not contain HCL interpolation or directive sequences")
	}

	// Walk the inner content to reject unescaped newlines and quotes.
	for i := 0; i < len(inner); i++ {
		ch := inner[i]
		if ch == '\\' {
			i++
			if i >= len(inner) {
				// Trailing backslash with no character to escape.
				// In HCL this would escape the closing quote delimiter,
				// producing an unterminated string.
				return xerrors.New("must not end with a trailing backslash")
			}
			continue
		}
		if ch == '"' {
			return xerrors.New("must not contain unescaped quotes")
		}
		if ch == '\n' || ch == '\r' {
			return xerrors.New("must not contain unescaped newlines")
		}
	}

	return nil
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
