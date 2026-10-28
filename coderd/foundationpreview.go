package coderd

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"path"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbauthz"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
)

func decodeFoundationPlanPreview(data json.RawMessage) *codersdk.FoundationPlanPreview {
	if len(data) == 0 {
		return nil
	}
	var preview codersdk.FoundationPlanPreview
	if json.Unmarshal(data, &preview) != nil {
		unavailable := provisionersdk.FoundationPlanPreview(nil)
		return &unavailable
	}
	preview.Advisory = provisionersdk.FoundationPreviewAdvisory
	preview.ConsumerReferences = []codersdk.FoundationConsumerReference{}
	if preview.OutputChanges == nil {
		preview.OutputChanges = []codersdk.FoundationOutputChange{}
	}
	return &preview
}

// enrichFoundationPreviews scans each version once, without exposing source or
// diagnostics. Caller must first authorize access to the Foundation. Consumer
// discovery includes hidden workspaces and therefore uses restricted system access.
func enrichFoundationPreviews(ctx context.Context, db database.Store, foundationID uuid.UUID, runs []codersdk.TemplateResourceRun) error {
	changed := map[string]bool{}
	for _, run := range runs {
		if run.Preview == nil || !run.Preview.Available {
			continue
		}
		for _, change := range run.Preview.OutputChanges {
			changed[change.Name] = true
		}
	}
	if len(changed) == 0 {
		return nil
	}
	ctx = dbauthz.AsSystemRestricted(ctx)
	versions, err := db.GetFoundationConsumerTemplateVersions(ctx, foundationID)
	if err != nil {
		return xerrors.Errorf("get Foundation consumer versions: %w", err)
	}
	type references struct {
		names      []string
		unresolved bool
	}
	byFile := map[uuid.UUID]references{}
	// Limit source analysis per response. Remaining versions are explicitly
	// unresolved instead of pretending the scan covered them.
	budget := 32 << 20
	for _, version := range versions {
		refs, ok := byFile[version.FileID]
		if !ok {
			refs.unresolved = true
			if budget > 0 && len(byFile) < 100 {
				file, err := db.GetFileByID(ctx, version.FileID)
				if err == nil && len(file.Data) <= budget {
					budget -= len(file.Data)
					refs.names, refs.unresolved = foundationSourceReferences(file.Data, changed)
				}
			}
			byFile[version.FileID] = refs
		}
		for _, run := range runs {
			if run.Preview == nil || !run.Preview.Available {
				continue
			}
			names := []string{}
			for _, change := range run.Preview.OutputChanges {
				if slices.Contains(refs.names, change.Name) {
					names = append(names, change.Name)
				}
			}
			if len(names) == 0 && !refs.unresolved {
				continue
			}
			run.Preview.ConsumerReferences = append(run.Preview.ConsumerReferences, codersdk.FoundationConsumerReference{
				TemplateVersionID: version.ID, OutputNames: names, Unresolved: refs.unresolved,
			})
		}
	}
	return nil
}

// foundationSourceReferences recognizes direct references and simple locals
// aliases, not Terraform's evaluation graph. Unsupported constructs remain
// advisory and unresolved. Only names already present in the plan can escape.
func foundationSourceReferences(archive []byte, changed map[string]bool) ([]string, bool) {
	// Reuse upload normalization's path, type, size, duplicate, and sparse-file
	// checks. Nothing is extracted to disk and errors must never be exposed.
	normalized, _, err := normalizeTemplateResourceArchive(archive, 32<<20)
	if err != nil {
		return nil, true
	}
	bodies := map[string][]*hclsyntax.Body{}
	unresolved := false
	reader := tar.NewReader(bytes.NewReader(normalized))
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, true
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		if strings.HasSuffix(header.Name, ".tf.json") {
			unresolved = true
			continue
		}
		if !strings.HasSuffix(header.Name, ".tf") {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(reader, templateResourceFileMaxBytes+1))
		if err != nil {
			return nil, true
		}
		file, diagnostics := hclsyntax.ParseConfig(data, "source.tf", hcl.InitialPos)
		if diagnostics.HasErrors() {
			unresolved = true
			continue
		}
		body, ok := file.Body.(*hclsyntax.Body)
		if !ok {
			unresolved = true
			continue
		}
		dir := path.Dir(header.Name)
		bodies[dir] = append(bodies[dir], body)
	}
	found := map[string]bool{}
	for dir, files := range bodies {
		// Local modules may receive outputs indirectly. Recognize their direct
		// references too, but never claim complete module/dataflow analysis.
		if dir != "." {
			unresolved = true
		}
		aliases := map[string]bool{}
		for _, body := range files {
			for _, block := range body.Blocks {
				if block.Type != "locals" {
					continue
				}
				for name, attr := range block.Body.Attributes {
					traversal, diagnostics := hcl.AbsTraversalForExpr(attr.Expr)
					if !diagnostics.HasErrors() && foundationOutputsTraversal(traversal) && len(traversal) == 4 {
						aliases[name] = true
					}
				}
			}
		}
		var scan func(*hclsyntax.Body, bool)
		scan = func(body *hclsyntax.Body, locals bool) {
			for name, attr := range body.Attributes {
				if locals && aliases[name] {
					continue
				}
				for _, traversal := range attr.Expr.Variables() {
					var tail hcl.Traversal
					switch {
					case foundationOutputsTraversal(traversal):
						tail = traversal[4:]
					case len(traversal) >= 2 && traversal.RootName() == "local":
						alias, ok := traversal[1].(hcl.TraverseAttr)
						if !ok || !aliases[alias.Name] {
							if ok && alias.Name == "foundation" {
								unresolved = true
							}
							continue
						}
						tail = traversal[2:]
					case len(traversal) >= 2 && traversal.RootName() == "data":
						kind, ok := traversal[1].(hcl.TraverseAttr)
						if ok && kind.Name == "coder_foundation" {
							unresolved = true
						}
						continue
					default:
						continue
					}
					if len(tail) == 0 {
						unresolved = true
						continue
					}
					output, ok := tail[0].(hcl.TraverseAttr)
					if !ok {
						unresolved = true
						continue
					}
					if changed[output.Name] {
						found[output.Name] = true
					}
				}
			}
			for _, block := range body.Blocks {
				if block.Type == "module" {
					unresolved = true
				}
				scan(block.Body, block.Type == "locals")
			}
		}
		for _, body := range files {
			scan(body, false)
		}
	}
	names := make([]string, 0, len(found))
	for name := range found {
		names = append(names, name)
	}
	slices.Sort(names)
	return names, unresolved
}

func foundationOutputsTraversal(traversal hcl.Traversal) bool {
	if len(traversal) < 4 || traversal.RootName() != "data" {
		return false
	}
	for i, name := range []string{"coder_foundation", "current"} {
		attr, ok := traversal[i+1].(hcl.TraverseAttr)
		if !ok || attr.Name != name {
			return false
		}
	}
	attr, ok := traversal[3].(hcl.TraverseAttr)
	return ok && (attr.Name == "outputs" || attr.Name == "sensitive_outputs")
}
