package templatebuilder

import (
	"embed"
	"io/fs"
)

// BaseOS enumerates operating systems for base template filtering.
type BaseOS string

const (
	BaseOSLinux   BaseOS = "linux"
	BaseOSWindows BaseOS = "windows"
)

// BaseTemplate holds metadata for a base template used by the template builder.
type BaseTemplate struct {
	ExampleID string
	OS        BaseOS
}

//go:embed bases
var basesFS embed.FS

const basesDir = "bases"

// BaseTemplates is the canonical map of example IDs to their OS.
// Maintained manually because TemplateExample.Tags are freeform and
// container/Kubernetes templates carry no OS tag.
var BaseTemplates = map[string]BaseTemplate{
	"docker":     {ExampleID: "docker", OS: BaseOSLinux},
	"kubernetes": {ExampleID: "kubernetes", OS: BaseOSLinux},
	"aws-linux":  {ExampleID: "aws-linux", OS: BaseOSLinux},
}

// BaseTemplateOS resolves the OS for a given example ID.
// Returns empty string if the example is not a known base template.
func BaseTemplateOS(exampleID string) BaseOS {
	bt, ok := BaseTemplates[exampleID]
	if !ok {
		return ""
	}
	return bt.OS
}

// DefaultBaseRenderContext returns the render context that produces the
// canonical default output for each base template.
func DefaultBaseRenderContext(exampleID string) BaseRenderContext {
	switch exampleID {
	case "docker":
		return BaseRenderContext{
			ContainerImage: "codercom/enterprise-base:ubuntu",
		}
	case "kubernetes":
		return BaseRenderContext{
			ContainerImage: "codercom/enterprise-base:ubuntu",
		}
	case "aws-linux":
		return BaseRenderContext{}
	default:
		return BaseRenderContext{}
	}
}

// BaseTemplateFS returns a filesystem rooted at the given base template
// directory within the embedded bases catalog.
func BaseTemplateFS(exampleID string) (fs.FS, error) {
	return fs.Sub(basesFS, basesDir+"/"+exampleID)
}
