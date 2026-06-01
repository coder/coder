package templatebuilder

import (
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
	files embed.FS

	catalogOnce    sync.Once
	catalogModules []ModuleManifest
	errCatalogLoad error
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
	PinnedVersion string           `json:"pinned_version"`
	Variables     []ModuleVariable `json:"variables"`
}

// ModuleVariable represents a variable declaration within a module manifest.
type ModuleVariable struct {
	Name           string  `json:"name"`
	Type           string  `json:"type"`
	Description    string  `json:"description"`
	Default        *string `json:"default,omitempty"`
	Required       bool    `json:"required"`
	Sensitive      bool    `json:"sensitive"`
	BuilderManaged bool    `json:"builder_managed"`
}

// LoadModules reads all module.json files from the embedded catalog.
// Results are cached after the first successful call.
func LoadModules() ([]ModuleManifest, error) {
	catalogOnce.Do(func() {
		catalogModules, errCatalogLoad = parseModules()
	})
	return catalogModules, errCatalogLoad
}

func parseModules() ([]ModuleManifest, error) {
	modulesFS, err := fs.Sub(files, modulesDir)
	if err != nil {
		return nil, xerrors.Errorf("get modules fs: %w", err)
	}

	dirs, err := fs.ReadDir(modulesFS, ".")
	if err != nil {
		return nil, xerrors.Errorf("read modules dir: %w", err)
	}

	var modules []ModuleManifest
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}

		manifestPath := path.Join(dir.Name(), "module.json")
		data, err := fs.ReadFile(modulesFS, manifestPath)
		if err != nil {
			// Skip directories without a module.json.
			continue
		}

		var manifest ModuleManifest
		if err := json.Unmarshal(data, &manifest); err != nil {
			return nil, xerrors.Errorf("decode %s: %w", manifestPath, err)
		}

		modules = append(modules, manifest)
	}

	return modules, nil
}

// ToSDK converts a ModuleManifest to the API response type.
// PinnedVersion is mapped to Version; tags are excluded from the
// API surface.
func (m ModuleManifest) ToSDK() codersdk.TemplateBuilderModule {
	variables := make([]codersdk.TemplateBuilderModuleVariable, 0, len(m.Variables))
	for _, v := range m.Variables {
		variables = append(variables, codersdk.TemplateBuilderModuleVariable{
			Name:           v.Name,
			Type:           codersdk.TemplateBuilderVariableType(v.Type),
			Description:    v.Description,
			Default:        v.Default,
			Required:       v.Required,
			Sensitive:      v.Sensitive,
			BuilderManaged: v.BuilderManaged,
		})
	}

	return codersdk.TemplateBuilderModule{
		ID:            m.ID,
		DisplayName:   m.DisplayName,
		Description:   m.Description,
		Icon:          m.Icon,
		Category:      m.Category,
		Version:       m.PinnedVersion,
		CompatibleOS:  m.CompatibleOS,
		ConflictsWith: m.ConflictsWith,
		Variables:     variables,
	}
}
