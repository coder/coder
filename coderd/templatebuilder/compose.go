package templatebuilder

import (
	"archive/tar"
	"bytes"
	"maps"
	"path"
	"slices"
	"time"

	"github.com/hashicorp/hcl/v2/hclwrite"
	"golang.org/x/xerrors"
)

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
	// ExtraFiles holds non-template files from the base directory
	// (e.g. cloud-init .tftpl files). Keys are paths relative to the
	// base directory.
	ExtraFiles map[string][]byte
}

// Compose renders a base template and selected modules into Terraform
// source files. It extracts the coder_agent resource name from the
// rendered base HCL and wires it into each module block.
func Compose(req ComposeRequest) (*ComposeResult, error) {
	mainTF, err := renderBase(req.BaseTemplateID, req.BaseVariableValues)
	if err != nil {
		return nil, err
	}

	extraFiles := BaseExtraFiles(req.BaseTemplateID)

	if len(req.Modules) == 0 {
		return &ComposeResult{
			MainTF:     formatHCL(mainTF),
			Readme:     []byte(BaseReadme(req.BaseTemplateID)),
			ExtraFiles: extraFiles,
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
		MainTF:     formatHCL(mainTF),
		ModulesTF:  formatHCL(modulesTF),
		Readme:     []byte(BaseReadme(req.BaseTemplateID)),
		ExtraFiles: extraFiles,
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

	// Write extra files in sorted order for reproducible archives.
	names := make([]string, 0, len(result.ExtraFiles))
	for name := range result.ExtraFiles {
		names = append(names, name)
	}
	slices.Sort(names)

	// Emit directory entries for any subdirectories so that
	// extractors that do not implicitly create parents can
	// unpack the archive.
	dirs := make(map[string]bool)
	for _, name := range names {
		for dir := path.Dir(name); dir != "." && !dirs[dir]; dir = path.Dir(dir) {
			dirs[dir] = true
		}
	}
	sortedDirs := make([]string, 0, len(dirs))
	for d := range dirs {
		sortedDirs = append(sortedDirs, d)
	}
	slices.Sort(sortedDirs)
	for _, d := range sortedDirs {
		if err := writeTarDir(tw, d); err != nil {
			return nil, xerrors.Errorf("write dir %s to tar: %w", d, err)
		}
	}

	for _, name := range names {
		if err := writeTarFile(tw, name, result.ExtraFiles[name]); err != nil {
			return nil, xerrors.Errorf("write %s to tar: %w", name, err)
		}
	}

	if err := tw.Close(); err != nil {
		return nil, xerrors.Errorf("close tar writer: %w", err)
	}

	return buf.Bytes(), nil
}

// writeTarDir adds a directory entry to a tar writer.
func writeTarDir(tw *tar.Writer, name string) error {
	hdr := &tar.Header{
		Typeflag: tar.TypeDir,
		Name:     name + "/",
		Mode:     0o755,
		ModTime:  time.Unix(0, 0),
	}
	return tw.WriteHeader(hdr)
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
