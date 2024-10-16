package workspacetags

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"slices"
	"strings"

	"cdr.dev/slog"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/terraform-config-inspect/tfconfig"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"
)

// Validate ensures that any uses of the `coder_workspace_tags` data source only
// reference the following data types:
// 1. Static variables
// 2. Template variables
// 3. Coder parameters
// Any other data types are not allowed, as their values cannot be known at
// the time of template import.
func Validate(ctx context.Context, logger slog.Logger, file []byte, mimetype string) (tags map[string]string, err error) {
	// TODO(cian): logic already exists in provisioner/terraform/parse.go for
	// this. Can we reuse it?

	// TODO(cian): we need to detect if there are missing UserVariableValues.
	// This is normally done by the provisioner, but we need to do it here.

	memfs := afero.NewMemMapFs()
	switch mimetype {
	case "application/x-tar":
		// untar the file into the filesystem
		if err := untarFS(memfs, file); err != nil {
			return nil, xerrors.Errorf("untar file: %w", err)
		}
	case "application/zip":
		// TODO: convert to tar and untar
		return nil, xerrors.New("zip files are not supported (yet)")
	default:
		return nil, xerrors.Errorf("unsupported mimetype: %s", mimetype)
	}
	tfMemFS, ok := memfs.(tfconfig.FS)
	if !ok {
		return nil, xerrors.New("memfs is not a tfconfig.FS")
	}
	module, diags := tfconfig.LoadModuleFromFilesystem(tfMemFS, "/")
	if diags.HasErrors() {
		return nil, xerrors.Errorf("load module: %s", diags.Error())
	}
	tags, err = loadWorkspaceTags(ctx, logger, module)
	return tags, err
}

// --- BEGIN COPYPASTA FROM provisioner/terraform/parse.go --- //
func loadWorkspaceTags(ctx context.Context, logger slog.Logger, module *tfconfig.Module) (map[string]string, error) {
	workspaceTags := map[string]string{}

	for _, dataResource := range module.DataResources {
		if dataResource.Type != "coder_workspace_tags" {
			logger.Debug(ctx, "skip resource as it is not a coder_workspace_tags", "resource_name", dataResource.Name, "resource_type", dataResource.Type)
			continue
		}

		var file *hcl.File
		var diags hcl.Diagnostics
		parser := hclparse.NewParser()

		if !strings.HasSuffix(dataResource.Pos.Filename, ".tf") {
			logger.Debug(ctx, "only .tf files can be parsed", "filename", dataResource.Pos.Filename)
			continue
		}
		// We know in which HCL file is the data resource defined.
		file, diags = parser.ParseHCLFile(dataResource.Pos.Filename)

		if diags.HasErrors() {
			return nil, xerrors.Errorf("can't parse the resource file: %s", diags.Error())
		}

		// Parse root to find "coder_workspace_tags".
		content, _, diags := file.Body.PartialContent(rootTemplateSchema)
		if diags.HasErrors() {
			return nil, xerrors.Errorf("can't parse the resource file: %s", diags.Error())
		}

		// Iterate over blocks to locate the exact "coder_workspace_tags" data resource.
		for _, block := range content.Blocks {
			if !slices.Equal(block.Labels, []string{"coder_workspace_tags", dataResource.Name}) {
				continue
			}

			// Parse "coder_workspace_tags" to find all key-value tags.
			resContent, _, diags := block.Body.PartialContent(coderWorkspaceTagsSchema)
			if diags.HasErrors() {
				return nil, xerrors.Errorf(`can't parse the resource coder_workspace_tags: %s`, diags.Error())
			}

			if resContent == nil {
				continue // workspace tags are not present
			}

			if _, ok := resContent.Attributes["tags"]; !ok {
				return nil, xerrors.Errorf(`"tags" attribute is required by coder_workspace_tags`)
			}

			expr := resContent.Attributes["tags"].Expr
			tagsExpr, ok := expr.(*hclsyntax.ObjectConsExpr)
			if !ok {
				return nil, xerrors.Errorf(`"tags" attribute is expected to be a key-value map`)
			}

			// Parse key-value entries in "coder_workspace_tags"
			for _, tagItem := range tagsExpr.Items {
				key, err := previewFileContent(tagItem.KeyExpr.Range())
				if err != nil {
					return nil, xerrors.Errorf("can't preview the resource file: %v", err)
				}
				key = strings.Trim(key, `"`)

				value, err := previewFileContent(tagItem.ValueExpr.Range())
				if err != nil {
					return nil, xerrors.Errorf("can't preview the resource file: %v", err)
				}

				logger.Info(ctx, "workspace tag found", "key", key, "value", value)

				if _, ok := workspaceTags[key]; ok {
					return nil, xerrors.Errorf(`workspace tag "%s" is defined multiple times`, key)
				}
				workspaceTags[key] = value
			}
		}
	}
	return workspaceTags, nil
}

func previewFileContent(fileRange hcl.Range) (string, error) {
	body, err := os.ReadFile(fileRange.Filename)
	if err != nil {
		return "", err
	}
	return string(fileRange.SliceBytes(body)), nil
}

var rootTemplateSchema = &hcl.BodySchema{
	Blocks: []hcl.BlockHeaderSchema{
		{
			Type:       "data",
			LabelNames: []string{"type", "name"},
		},
	},
}

var coderWorkspaceTagsSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{
		{
			Name: "tags",
		},
	},
}

func untarFS(fs afero.Fs, tarball []byte) error {
	tr := tar.NewReader(bytes.NewReader(tarball))
	for {
		th, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return xerrors.Errorf("read tar archive: %w", err)
		}
		switch th.Typeflag {
		case tar.TypeDir:
			if err := fs.MkdirAll(th.Name, 0755); err != nil {
				return xerrors.Errorf("mkdir %s: %w", th.Name, err)
			}
		case tar.TypeReg:
			f, err := fs.OpenFile(th.Name, os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				return xerrors.Errorf("open %s: %w", th.Name, err)
			}
			defer f.Close()
			if _, err := io.Copy(f, tr); err != nil {
				return xerrors.Errorf("copy %s: %w", th.Name, err)
			}
			f.Close()
		default:
			return xerrors.Errorf("unsupported type: %s", th.Typeflag)
		}
	}
	return nil
}
