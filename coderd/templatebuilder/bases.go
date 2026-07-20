package templatebuilder

import (
	"bytes"
	"embed"
	"encoding/json"
	"io/fs"
	"path"
	"strings"
	"sync"
	"text/template"

	"golang.org/x/xerrors"
)

// BaseOS enumerates operating systems for base template filtering.
type BaseOS string

const (
	BaseOSLinux   BaseOS = "linux"
	BaseOSWindows BaseOS = "windows"
)

// validBaseOS maps base.json os strings to their typed equivalents.
var validBaseOS = map[string]BaseOS{
	"linux":   BaseOSLinux,
	"windows": BaseOSWindows,
}

//go:embed bases
var basesFS embed.FS

const basesDir = "bases"

// templateSuffix identifies Go template files that are pre-parsed at load time.
// Terraform templatefile() inputs (.tftpl) are not Go templates and are left
// as raw files in the embedded FS.
const templateSuffix = ".tf.tmpl"

// BaseManifest is the on-disk schema for a base.json file.
type BaseManifest struct {
	ID             string             `json:"id"`
	DisplayName    string             `json:"display_name"`
	OS             string             `json:"os"`
	DefaultContext BaseDefaultContext `json:"default_context"`
	Variables      []ModuleVariable   `json:"variables"`
}

// BaseDefaultContext holds default render values stored in base.json.
type BaseDefaultContext struct {
	ContainerImage string `json:"container_image,omitempty"`
}

// parsedBase holds the result of loading and pre-parsing a single base
// template directory.
type parsedBase struct {
	Manifest      BaseManifest
	Templates     map[string]*template.Template
	FS            fs.FS
	Readme        string            // full README.md content (including frontmatter)
	Prerequisites string            // content between prerequisite comment markers
	ExtraFiles    map[string][]byte // non-template, non-manifest files (e.g. .tftpl)
}

var loadBases = sync.OnceValues(func() (map[string]*parsedBase, error) {
	return parseBasesFromFS(basesFS)
})

// parseBasesFromFS reads and validates all base.json manifests and pre-parses
// Go template files from the given filesystem. Most callers should use the
// exported accessors, which read from the cached embedded catalog.
func parseBasesFromFS(fsys fs.FS) (map[string]*parsedBase, error) {
	sub, err := fs.Sub(fsys, basesDir)
	if err != nil {
		return nil, xerrors.Errorf("open embedded base catalog: %w", err)
	}

	dirs, err := fs.ReadDir(sub, ".")
	if err != nil {
		return nil, xerrors.Errorf("list base catalog entries: %w", err)
	}

	bases := make(map[string]*parsedBase)
	for _, dir := range dirs {
		if !dir.IsDir() {
			continue
		}

		base, err := parseBaseDir(sub, dir.Name())
		if err != nil {
			return nil, err
		}
		if bases[base.Manifest.ID] != nil {
			return nil, xerrors.Errorf("duplicate base id %q", base.Manifest.ID)
		}
		bases[base.Manifest.ID] = base
	}

	return bases, nil
}

// parseBaseDir loads a single base template directory: reads the
// manifest, pre-parses Go templates, reads the README, and collects
// extra files.
func parseBaseDir(parent fs.FS, dirName string) (*parsedBase, error) {
	manifest, err := parseManifest(parent, dirName)
	if err != nil {
		return nil, err
	}

	baseFS, err := fs.Sub(parent, dirName)
	if err != nil {
		return nil, xerrors.Errorf("sub fs for %s: %w", dirName, err)
	}

	templates, err := parseTemplatesFromFS(baseFS)
	if err != nil {
		return nil, xerrors.Errorf("parse templates for base %q: %w", manifest.ID, err)
	}

	readmeData, err := fs.ReadFile(baseFS, "README.md")
	if err != nil {
		return nil, xerrors.Errorf("read README.md for base %q: %w", manifest.ID, err)
	}
	readme := string(readmeData)

	extraFiles, err := collectExtraFilesFromFS(baseFS)
	if err != nil {
		return nil, xerrors.Errorf("collect extra files for base %q: %w", manifest.ID, err)
	}

	return &parsedBase{
		Manifest:      manifest,
		Templates:     templates,
		FS:            baseFS,
		Readme:        readme,
		Prerequisites: ExtractPrerequisites(readme),
		ExtraFiles:    extraFiles,
	}, nil
}

// parseManifest reads and validates a base.json file from the given
// directory within parent.
func parseManifest(parent fs.FS, dirName string) (BaseManifest, error) {
	manifestPath := path.Join(dirName, "base.json")
	data, err := fs.ReadFile(parent, manifestPath)
	if err != nil {
		return BaseManifest{}, xerrors.Errorf("read %s: %w", manifestPath, err)
	}

	var manifest BaseManifest
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return BaseManifest{}, xerrors.Errorf("decode %s: %w", manifestPath, err)
	}

	if manifest.ID == "" {
		return BaseManifest{}, xerrors.Errorf("base in %s has empty id", dirName)
	}
	if _, ok := validBaseOS[manifest.OS]; !ok && manifest.OS != "" {
		return BaseManifest{}, xerrors.Errorf("base %q has unknown os %q", manifest.ID, manifest.OS)
	}

	return manifest, nil
}

// parseTemplatesFromFS walks the filesystem and pre-parses all .tf.tmpl files
// into Go templates. Returned keys are paths relative to the FS root.
func parseTemplatesFromFS(fsys fs.FS) (map[string]*template.Template, error) {
	templates := make(map[string]*template.Template)

	err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(p, templateSuffix) {
			return nil
		}

		raw, err := fs.ReadFile(fsys, p)
		if err != nil {
			return xerrors.Errorf("read %s: %w", p, err)
		}

		tmpl, err := template.New(p).Parse(string(raw))
		if err != nil {
			return xerrors.Errorf("parse %s: %w", p, err)
		}

		templates[p] = tmpl
		return nil
	})
	if err != nil {
		return nil, err
	}

	return templates, nil
}

// BaseTemplateOS resolves the OS for a given example ID.
// Returns empty string if the example is not a known base template.
func BaseTemplateOS(exampleID string) BaseOS {
	bases, err := loadBases()
	if err != nil || bases[exampleID] == nil {
		return ""
	}
	return validBaseOS[bases[exampleID].Manifest.OS]
}

// DefaultBaseRenderContext returns the render context that produces the
// canonical default output for a base template.
func DefaultBaseRenderContext(exampleID string) BaseRenderContext {
	bases, err := loadBases()
	if err != nil || bases[exampleID] == nil {
		return BaseRenderContext{}
	}
	base := bases[exampleID]
	dc := base.Manifest.DefaultContext

	// Populate Variables from manifest defaults so that Go template
	// rendering succeeds even without caller-supplied values.
	vars := make(map[string]string, len(base.Manifest.Variables))
	for _, v := range base.Manifest.Variables {
		if v.Computed || v.Sensitive {
			continue
		}
		if len(v.Default) > 0 && isSimpleJSONValue(v.Default) {
			vars[v.Name] = string(v.Default)
		}
	}

	return BaseRenderContext{
		ContainerImage: dc.ContainerImage,
		Variables:      vars,
	}
}

// BaseTemplateIDs returns the set of known base template example IDs.
func BaseTemplateIDs() []string {
	bases, err := loadBases()
	if err != nil {
		return nil
	}
	ids := make([]string, 0, len(bases))
	for id := range bases {
		ids = append(ids, id)
	}
	return ids
}

// BaseVariables returns the user-facing variables for a given base
// template ID. Computed variables are excluded. Returns nil if the
// base is unknown or has no variables.
func BaseVariables(exampleID string) []ModuleVariable {
	bases, err := loadBases()
	if err != nil || bases[exampleID] == nil {
		return nil
	}
	return bases[exampleID].Manifest.Variables
}

// BaseTemplateFS returns a filesystem rooted at the given base template
// directory within the embedded bases catalog. Returns an error if
// exampleID is not a known base template.
func BaseTemplateFS(exampleID string) (fs.FS, error) {
	bases, err := loadBases()
	if err != nil {
		return nil, xerrors.Errorf("load base catalog: %w", err)
	}
	base, ok := bases[exampleID]
	if !ok {
		return nil, xerrors.Errorf("unknown base template %q", exampleID)
	}
	return base.FS, nil
}

// BaseReadme returns the full README.md content for a base template.
// Returns an empty string if the base is unknown or has no README.
func BaseReadme(exampleID string) string {
	bases, err := loadBases()
	if err != nil || bases[exampleID] == nil {
		return ""
	}
	return bases[exampleID].Readme
}

// BasePrerequisites returns the prerequisites section extracted from
// the base template README. Returns an empty string if the base is
// unknown or has no prerequisites markers.
func BasePrerequisites(exampleID string) string {
	bases, err := loadBases()
	if err != nil || bases[exampleID] == nil {
		return ""
	}
	return bases[exampleID].Prerequisites
}

// BaseExtraFiles returns the non-template, non-manifest files embedded
// in the base template directory (e.g. cloud-init .tftpl files). Returns
// nil if the base is unknown or has no extra files.
func BaseExtraFiles(exampleID string) map[string][]byte {
	bases, err := loadBases()
	if err != nil || bases[exampleID] == nil {
		return nil
	}
	return bases[exampleID].ExtraFiles
}

// collectExtraFilesFromFS walks a base template filesystem and returns
// all files that are not Go templates (.tf.tmpl), the manifest
// (base.json), or the README. These are raw files that must be included
// in the output archive (e.g. Terraform templatefile() inputs).
func collectExtraFilesFromFS(baseFS fs.FS) (map[string][]byte, error) {
	files := make(map[string][]byte)
	err := fs.WalkDir(baseFS, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if p == "base.json" || p == "README.md" || strings.HasSuffix(p, ".tf.tmpl") {
			return nil
		}
		data, err := fs.ReadFile(baseFS, p)
		if err != nil {
			return xerrors.Errorf("read %s: %w", p, err)
		}
		files[p] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return files, nil
}
