package templatebuilder

import (
	"bytes"

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

// RenderBaseTemplate executes a pre-parsed .tf.tmpl template for the given
// base, applying the provided render context. Templates are parsed once at
// startup; parse errors surface on first access rather than at render time.
func RenderBaseTemplate(exampleID, templatePath string, renderCtx BaseRenderContext) ([]byte, error) {
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
