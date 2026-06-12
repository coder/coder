package templatebuilder

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"time"

	"golang.org/x/xerrors"
)

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
		return &ComposeResult{MainTF: mainTF}, nil
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
		MainTF:    mainTF,
		ModulesTF: modulesTF,
	}, nil
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

		vars := mergeModuleVariables(manifest, cm.Variables)
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
// variables, then overlays caller-supplied values. This ensures every
// variable referenced in the template has a value.
func mergeModuleVariables(manifest ModuleManifest, callerVars map[string]string) map[string]string {
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
	for k, val := range callerVars {
		merged[k] = val
	}
	return merged
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
