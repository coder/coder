package templatebuilder

import (
	"embed"
	"io/fs"

	"golang.org/x/xerrors"
)

// BaseOS enumerates operating systems for base template filtering.
type BaseOS string

const (
	BaseOSLinux BaseOS = "linux"
)

// BaseTemplate holds metadata for a base template used by the template builder.
type BaseTemplate struct {
	OS             BaseOS
	DefaultContext BaseRenderContext
}

//go:embed bases
var basesFS embed.FS

const basesDir = "bases"

// baseTemplates is the canonical map of example IDs to their metadata.
// Maintained manually because TemplateExample.Tags are freeform and
// container/Kubernetes templates carry no OS tag.
// Do not mutate this map; use the exported accessors to read from it.
var baseTemplates = map[string]BaseTemplate{
	"docker": {
		OS: BaseOSLinux,
		DefaultContext: BaseRenderContext{
			ContainerImage: "codercom/enterprise-base:ubuntu",
		},
	},
	"kubernetes": {
		OS: BaseOSLinux,
		DefaultContext: BaseRenderContext{
			ContainerImage: "codercom/enterprise-base:ubuntu",
		},
	},
	"aws-linux": {
		OS: BaseOSLinux,
	},
}

// BaseTemplateOS resolves the OS for a given example ID.
// Returns empty string if the example is not a known base template.
func BaseTemplateOS(exampleID string) BaseOS {
	bt, ok := baseTemplates[exampleID]
	if !ok {
		return ""
	}
	return bt.OS
}

// DefaultBaseRenderContext returns the render context that produces the
// canonical default output for a base template.
func DefaultBaseRenderContext(exampleID string) BaseRenderContext {
	bt, ok := baseTemplates[exampleID]
	if !ok {
		return BaseRenderContext{}
	}
	return bt.DefaultContext
}

// BaseTemplateIDs returns the set of known base template example IDs.
func BaseTemplateIDs() []string {
	ids := make([]string, 0, len(baseTemplates))
	for id := range baseTemplates {
		ids = append(ids, id)
	}
	return ids
}

// BaseTemplateFS returns a filesystem rooted at the given base template
// directory within the embedded bases catalog. Returns an error if
// exampleID is not a known base template.
func BaseTemplateFS(exampleID string) (fs.FS, error) {
	if _, ok := baseTemplates[exampleID]; !ok {
		return nil, xerrors.Errorf("unknown base template %q", exampleID)
	}
	return fs.Sub(basesFS, basesDir+"/"+exampleID)
}
