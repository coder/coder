package templatebuilder

import (
	"bytes"
	"io/fs"
	"text/template"

	"golang.org/x/xerrors"
)

// ImageOption represents a container image choice for base template parameters.
type ImageOption struct {
	Name  string // display name for the dropdown
	Value string // container image identifier
}

// BaseRenderContext is the data passed to base template .tf.tmpl files.
type BaseRenderContext struct {
	ContainerImage string            // selected image, becomes coder_parameter default
	ImageOptions   []ImageOption     // curated list, becomes coder_parameter options
	Variables      map[string]string // base_variable_values from compose request
}

// RenderBaseTemplate parses and executes a .tf.tmpl file from the given
// filesystem, applying the provided render context. The templatePath
// should be relative to fsys (e.g. "main.tf.tmpl").
func RenderBaseTemplate(fsys fs.FS, templatePath string, ctx BaseRenderContext) ([]byte, error) {
	data, err := fs.ReadFile(fsys, templatePath)
	if err != nil {
		return nil, xerrors.Errorf("read template %s: %w", templatePath, err)
	}

	tmpl, err := template.New(templatePath).Parse(string(data))
	if err != nil {
		return nil, xerrors.Errorf("parse template %s: %w", templatePath, err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, ctx); err != nil {
		return nil, xerrors.Errorf("execute template %s: %w", templatePath, err)
	}

	return buf.Bytes(), nil
}
