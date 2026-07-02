package templatebuilder

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"path"
	"sync"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
)

var (
	//go:embed modules
	modulesFS embed.FS

	loadModules = sync.OnceValues(func() ([]ModuleManifest, error) {
		return parseModulesFromFS(modulesFS)
	})
)

const modulesDir = "modules"

// ModuleManifest represents a module.json file from the bundled catalog.
// This is the on-disk schema; codersdk.TemplateBuilderModule is the API type.
type ModuleManifest struct {
	ID            string           `json:"id"`
	DisplayName   string           `json:"display_name"`
	Description   string           `json:"description"`
	Icon          string           `json:"icon"`
	Category      string           `json:"category"`
	Tags          []string         `json:"tags"`
	CompatibleOS  []string         `json:"compatible_os"`
	ConflictsWith []string         `json:"conflicts_with"`
	Namespace     string           `json:"namespace"`
	PinnedVersion string           `json:"pinned_version"`
	Variables     []ModuleVariable `json:"variables"`
}

// ModuleVariable represents a variable declaration within a module manifest.
type ModuleVariable struct {
	Name        string          `json:"name"`
	Type        string          `json:"type"`
	Description string          `json:"description"`
	Default     json.RawMessage `json:"default,omitempty"`
	Required    bool            `json:"required"`
	Sensitive   bool            `json:"sensitive"`
	Computed    bool            `json:"computed"`
}

// validVariableTypes maps module.json type strings to their SDK equivalents.
// Used both for validation in parseModulesFromFS and for conversion in ToSDK.
var validVariableTypes = map[string]codersdk.TemplateBuilderVariableType{
	"string": codersdk.TemplateBuilderVariableTypeString,
	"number": codersdk.TemplateBuilderVariableTypeNumber,
	"bool":   codersdk.TemplateBuilderVariableTypeBool,
}

// LoadModules returns all module manifests from the embedded catalog.
// Results are cached after the first call, including errors. Each call
// returns a fresh slice so callers can filter or sort without corrupting
// the cache.
func LoadModules() ([]ModuleManifest, error) {
	modules, err := loadModules()
	if err != nil {
		return nil, err
	}
	out := make([]ModuleManifest, len(modules))
	copy(out, modules)
	return out, nil
}

// parseModulesFromFS reads and validates all module.json files from the
// given filesystem. Most callers should use LoadModules, which reads from
// the embedded catalog.
func parseModulesFromFS(fsys fs.FS) ([]ModuleManifest, error) {
	sub, err := fs.Sub(fsys, modulesDir)
	if err != nil {
		return nil, xerrors.Errorf("open embedded module catalog: %w", err)
	}

	dirs, err := fs.ReadDir(sub, ".")
	if err != nil {
		return nil, xerrors.Errorf("list module catalog entries: %w", err)
	}

	seen := make(map[string]bool)
	var modules []ModuleManifest
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}

		manifestPath := path.Join(dir.Name(), "module.json")
		data, err := fs.ReadFile(sub, manifestPath)
		if err != nil {
			return nil, xerrors.Errorf("read %s: %w", manifestPath, err)
		}

		var manifest ModuleManifest
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&manifest); err != nil {
			return nil, xerrors.Errorf("decode %s: %w", manifestPath, err)
		}

		if manifest.ID == "" {
			return nil, xerrors.Errorf("module in %s has empty id", dir.Name())
		}
		if manifest.PinnedVersion == "" {
			return nil, xerrors.Errorf("module %q has empty pinned_version", manifest.ID)
		}
		if seen[manifest.ID] {
			return nil, xerrors.Errorf("duplicate module id %q", manifest.ID)
		}
		seen[manifest.ID] = true

		seenVars := make(map[string]bool)
		for i, v := range manifest.Variables {
			if v.Name == "" {
				return nil, xerrors.Errorf("module %q variable %d has empty name", manifest.ID, i)
			}
			if seenVars[v.Name] {
				return nil, xerrors.Errorf("module %q has duplicate variable name %q", manifest.ID, v.Name)
			}
			seenVars[v.Name] = true
			if _, ok := validVariableTypes[v.Type]; !ok {
				return nil, xerrors.Errorf("module %q variable %d (%q): unknown type %q", manifest.ID, i, v.Name, v.Type)
			}
		}

		modules = append(modules, manifest)
	}

	return modules, nil
}

// CompatibleWithOS reports whether the module is compatible with the given OS.
// Modules with an empty CompatibleOS list are compatible with all platforms.
func (m ModuleManifest) CompatibleWithOS(os string) bool {
	if len(m.CompatibleOS) == 0 {
		return true
	}
	for _, supported := range m.CompatibleOS {
		if supported == os {
			return true
		}
	}
	return false
}

// ToSDK converts a ModuleManifest to the API response type.
// PinnedVersion is mapped to Version; tags are not part of the API surface.
// Computed variables are excluded from the output.
func (m ModuleManifest) ToSDK() codersdk.TemplateBuilderModule {
	variables := make([]codersdk.TemplateBuilderModuleVariable, 0, len(m.Variables))
	for _, v := range m.Variables {
		// Computed variables (e.g. agent_id) are wired by the builder
		// automatically and must not be surfaced to the user.
		if v.Computed {
			continue
		}
		variables = append(variables, codersdk.TemplateBuilderModuleVariable{
			Name:        v.Name,
			Type:        validVariableTypes[v.Type],
			Description: v.Description,
			Default:     v.Default,
			Required:    v.Required,
			Sensitive:   v.Sensitive,
		})
	}

	// CLEANUP: json/v2
	compatibleOS := m.CompatibleOS
	if compatibleOS == nil {
		compatibleOS = []string{}
	}
	conflictsWith := m.ConflictsWith
	if conflictsWith == nil {
		conflictsWith = []string{}
	}

	return codersdk.TemplateBuilderModule{
		ID:            m.ID,
		DisplayName:   m.DisplayName,
		Description:   m.Description,
		Icon:          m.Icon,
		Category:      m.Category,
		Version:       m.PinnedVersion,
		CompatibleOS:  compatibleOS,
		ConflictsWith: conflictsWith,
		Variables:     variables,
	}
}

// ModuleTemplateFS returns an fs.FS rooted at the embedded directory for
// the given module ID, providing access to its .tf.tmpl file.
func ModuleTemplateFS(moduleID string) (fs.FS, error) {
	modPath := modulesDir + "/" + moduleID
	// Verify the directory exists. fs.Sub on embed.FS silently succeeds
	// for nonexistent paths, so we check for the expected .tf.tmpl file.
	tmplName := moduleID + ".tf.tmpl"
	if _, err := fs.Stat(modulesFS, modPath+"/"+tmplName); err != nil {
		return nil, xerrors.Errorf("module %q not found in embedded catalog: %w", moduleID, err)
	}
	sub, err := fs.Sub(modulesFS, modPath)
	if err != nil {
		return nil, xerrors.Errorf("module %q sub-filesystem: %w", moduleID, err)
	}
	return sub, nil
}
