package cli

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hexops/gotextdiff"
	"github.com/hexops/gotextdiff/myers"
	"github.com/hexops/gotextdiff/span"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/provisionersdk"
	"github.com/coder/pretty"
	"github.com/coder/serpent"
)

// templateVersionsDiff compares two template versions.
// Initial implementation generated with assistance from Mux (mux.coder.com).
func (r *RootCmd) templateVersionsDiff() *serpent.Command {
	var (
		versionFrom string
		versionTo   string
		orgContext  = NewOrganizationContext()
	)

	cmd := &serpent.Command{
		Use:   "diff <template>",
		Short: "Compare two versions of a template",
		Long: FormatExamples(
			Example{
				Description: "Compare two specific versions of a template",
				Command:     "coder templates versions diff my-template --from v1 --to v2",
			},
			Example{
				Description: "Compare a version against the active version",
				Command:     "coder templates versions diff my-template --from v1",
			},
			Example{
				Description: "Interactive: select versions to compare",
				Command:     "coder templates versions diff my-template",
			},
		),
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			templateName := inv.Args[0]

			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return xerrors.Errorf("get current organization: %w", err)
			}

			template, err := client.TemplateByName(ctx, organization.ID, templateName)
			if err != nil {
				return xerrors.Errorf("get template by name: %w", err)
			}

			// Get all versions
			versions, err := client.TemplateVersionsByTemplate(ctx, codersdk.TemplateVersionsByTemplateRequest{
				TemplateID: template.ID,
			})
			if err != nil {
				return xerrors.Errorf("get template versions: %w", err)
			}

			if len(versions) < 2 {
				return xerrors.Errorf("template %q has fewer than 2 versions, nothing to compare", templateName)
			}

			// Sort versions by creation date (newest first)
			sort.SliceStable(versions, func(i, j int) bool {
				return versions[i].CreatedAt.After(versions[j].CreatedAt)
			})

			// Build version names list for interactive selection
			var versionNames []string
			for _, v := range versions {
				label := v.Name
				if v.ID == template.ActiveVersionID {
					label += " (active)"
				}
				versionNames = append(versionNames, label)
			}

			// Resolve "from" version
			if versionFrom == "" {
				// Interactive selection
				selected, err := cliui.Select(inv, cliui.SelectOptions{
					Options: versionNames,
					Message: "Select the first version (older/base):",
				})
				if err != nil {
					return err
				}
				versionFrom = strings.TrimSuffix(selected, " (active)")
			}

			// Resolve "to" version (defaults to active)
			if versionTo == "" {
				// Interactive selection or default to active
				if inv.Stdin == os.Stdin {
					selected, err := cliui.Select(inv, cliui.SelectOptions{
						Options: versionNames,
						Message: "Select the second version (newer/target):",
					})
					if err != nil {
						return err
					}
					versionTo = strings.TrimSuffix(selected, " (active)")
				} else {
					// Non-interactive: default to active version
					versionTo = "active"
				}
			}

			// Fetch full version details using TemplateVersionByName (like templatepull does)
			var fromVersion, toVersion codersdk.TemplateVersion

			fromVersion, err = client.TemplateVersionByName(ctx, template.ID, versionFrom)
			if err != nil {
				return xerrors.Errorf("get version %q: %w", versionFrom, err)
			}

			if versionTo == "active" {
				toVersion, err = client.TemplateVersion(ctx, template.ActiveVersionID)
			} else {
				toVersion, err = client.TemplateVersionByName(ctx, template.ID, versionTo)
			}
			if err != nil {
				return xerrors.Errorf("get version %q: %w", versionTo, err)
			}

			if fromVersion.ID == toVersion.ID {
				cliui.Info(inv.Stderr, "Both versions are the same, no diff to show.")
				return nil
			}

			cliui.Info(inv.Stderr, fmt.Sprintf("Comparing %s → %s", cliui.Bold(fromVersion.Name), cliui.Bold(toVersion.Name)))

			// Download both versions
			fromFiles, err := downloadAndExtractVersion(ctx, client, fromVersion)
			if err != nil {
				return xerrors.Errorf("download version %q: %w", fromVersion.Name, err)
			}

			toFiles, err := downloadAndExtractVersion(ctx, client, toVersion)
			if err != nil {
				return xerrors.Errorf("download version %q: %w", toVersion.Name, err)
			}

			// Generate diff
			diff := generateDiff(fromVersion.Name, toVersion.Name, fromFiles, toFiles)

			if diff == "" {
				cliui.Info(inv.Stderr, "No differences found between versions.")
				return nil
			}

			// Output colorized diff
			_, _ = fmt.Fprintln(inv.Stdout, colorizeDiff(diff))
			return nil
		},
	}

	cmd.Options = serpent.OptionSet{
		{
			Description: "The base version to compare from.",
			Flag:        "from",
			Value:       serpent.StringOf(&versionFrom),
		},
		{
			Description: "The target version to compare to (defaults to active version).",
			Flag:        "to",
			Value:       serpent.StringOf(&versionTo),
		},
	}
	orgContext.AttachOptions(cmd)

	return cmd
}

// downloadAndExtractVersion downloads a template version and extracts its files into memory
func downloadAndExtractVersion(ctx context.Context, client *codersdk.Client, version codersdk.TemplateVersion) (map[string]string, error) {
	raw, ctype, err := client.DownloadWithFormat(ctx, version.Job.FileID, "")
	if err != nil {
		return nil, xerrors.Errorf("download: %w", err)
	}

	if ctype != codersdk.ContentTypeTar {
		return nil, xerrors.Errorf("unexpected content type %q", ctype)
	}

	// Extract to temp directory
	tmpDir, err := os.MkdirTemp("", "coder-template-diff-*")
	if err != nil {
		return nil, xerrors.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	err = provisionersdk.Untar(tmpDir, bytes.NewReader(raw))
	if err != nil {
		return nil, xerrors.Errorf("untar: %w", err)
	}

	// Read all files into memory
	files := make(map[string]string)
	err = filepath.Walk(tmpDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		relPath, err := filepath.Rel(tmpDir, path)
		if err != nil {
			return err
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}

		files[relPath] = string(content)
		return nil
	})
	if err != nil {
		return nil, xerrors.Errorf("walk files: %w", err)
	}

	return files, nil
}

// generateDiff creates a unified diff between two sets of files
func generateDiff(fromName, toName string, fromFiles, toFiles map[string]string) string {
	// Collect all unique file paths
	allPaths := make(map[string]struct{})
	for p := range fromFiles {
		allPaths[p] = struct{}{}
	}
	for p := range toFiles {
		allPaths[p] = struct{}{}
	}

	// Sort paths for deterministic output
	var paths []string
	for p := range allPaths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var result strings.Builder
	for _, path := range paths {
		fromContent := fromFiles[path]
		toContent := toFiles[path]

		if fromContent == toContent {
			continue
		}

		fromLabel := fmt.Sprintf("a/%s (%s)", path, fromName)
		toLabel := fmt.Sprintf("b/%s (%s)", path, toName)

		edits := myers.ComputeEdits(span.URIFromPath(path), fromContent, toContent)
		unified := gotextdiff.ToUnified(fromLabel, toLabel, fromContent, edits)

		if len(unified.Hunks) > 0 {
			result.WriteString(fmt.Sprint(unified))
			result.WriteString("\n")
		}
	}

	return result.String()
}

// colorizeDiff adds ANSI colors to diff output
func colorizeDiff(diff string) string {
	var result strings.Builder
	lines := strings.Split(diff, "\n")

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "+++") || strings.HasPrefix(line, "---"):
			result.WriteString(pretty.Sprint(cliui.DefaultStyles.Code, line))
		case strings.HasPrefix(line, "+"):
			result.WriteString(pretty.Sprint(cliui.DefaultStyles.Keyword, line))
		case strings.HasPrefix(line, "-"):
			result.WriteString(pretty.Sprint(cliui.DefaultStyles.Error, line))
		case strings.HasPrefix(line, "@@"):
			result.WriteString(pretty.Sprint(cliui.DefaultStyles.Placeholder, line))
		default:
			result.WriteString(line)
		}
		result.WriteString("\n")
	}

	return strings.TrimSuffix(result.String(), "\n")
}
