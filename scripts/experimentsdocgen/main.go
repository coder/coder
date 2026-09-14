// Command experimentsdocgen generates the experiments reference at
// docs/reference/experiments.md from codersdk.
//
// Two sources are combined. The set of experiments, their display names, and
// whether each one is safe to enable through the wildcard come from the
// codersdk package at run time. The per-experiment descriptions live as
// trailing comments on the Experiment constants, so they are read from the
// syntax tree of the file that declares them.
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	utilstrings "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scripts/atomicwrite"
	"github.com/coder/coder/v2/scripts/docgenenv"
	"github.com/coder/flog"
)

// routeTitles is the manifest breadcrumb to this page's route. The page
// mirrors that route's metadata into its front matter, so the manifest stays
// the single source of the title and description.
var routeTitles = []string{"Reference", "Experiments"}

const intro = `An experiment is a feature that is not ready for production.
Experiments are disabled by default, are not guaranteed to be backward compatible, and can be renamed or removed at any time.

Enable one by passing its key to ` + "`coder server`" + `:

` + "```shell" + `
coder server --experiments=<experiment-key>
` + "```" + `

The same keys work through the ` + "`CODER_EXPERIMENTS`" + ` environment variable.
For how experiments relate to beta and generally available features, refer to [Feature stages](../install/releases/feature-stages.md).

`

const wildcardSection = `## The wildcard value

` + "`--experiments=*`" + ` enables only the experiments Coder marks as safe for general opt-in, not every experiment on this page.

`

const tableSection = `## Available experiments

`

func main() {
	manifestPath := flag.String("manifest", "docs/manifest.json", "path to the docs manifest that supplies the page metadata")
	source := flag.String("source", "codersdk/deployment.go", "path to the Go file declaring the Experiment constants")
	out := flag.String("out", "docs/reference/experiments.md", "path to write the generated reference page")
	flag.Parse()

	manifest, err := docgenenv.LoadManifest(*manifestPath)
	if err != nil {
		flog.Fatalf("%v", err)
	}
	route := manifest.FindRoute(routeTitles...)
	if route == nil {
		flog.Fatalf("manifest %q has no route %q", *manifestPath, strings.Join(routeTitles, " > "))
	}

	descriptions, err := readDescriptions(*source)
	if err != nil {
		flog.Fatalf("%v", err)
	}
	displayNames, err := readDisplayNames(*source)
	if err != nil {
		flog.Fatalf("%v", err)
	}

	content, err := render(*route, codersdk.ExperimentsKnown, codersdk.ExperimentsSafe, descriptions, displayNames)
	if err != nil {
		flog.Fatalf("render experiments reference: %v", err)
	}
	if err := atomicwrite.File(*out, []byte(content)); err != nil {
		flog.Fatalf("write %s: %v", *out, err)
	}
	flog.Successf("wrote %s", *out)
}

// readDescriptions returns the trailing comment on each Experiment constant,
// keyed by the experiment's string value.
func readDescriptions(path string) (map[string]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, xerrors.Errorf("parse %q: %w", path, err)
	}

	descriptions := map[string]string{}
	for node := range ast.Preorder(file) {
		spec, ok := node.(*ast.ValueSpec)
		if !ok {
			continue
		}
		ident, ok := spec.Type.(*ast.Ident)
		if !ok || ident.Name != "Experiment" || len(spec.Values) != 1 {
			continue
		}
		lit, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			continue
		}
		descriptions[value] = commentSentence(spec)
	}

	if len(descriptions) == 0 {
		return nil, xerrors.Errorf("no Experiment constants found in %q", path)
	}
	return descriptions, nil
}

func readDisplayNames(path string) (map[string]bool, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return nil, xerrors.Errorf("parse %q: %w", path, err)
	}

	constants := map[string]string{}
	for node := range ast.Preorder(file) {
		spec, ok := node.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			continue
		}
		ident, ok := spec.Type.(*ast.Ident)
		if !ok || ident.Name != "Experiment" {
			continue
		}
		lit, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			continue
		}
		constants[spec.Names[0].Name] = value
	}

	displayNames := map[string]bool{}
	for node := range ast.Preorder(file) {
		decl, ok := node.(*ast.FuncDecl)
		if !ok || decl.Name.Name != "DisplayName" || decl.Recv == nil || len(decl.Recv.List) != 1 {
			continue
		}
		receiver, ok := decl.Recv.List[0].Type.(*ast.Ident)
		if !ok || receiver.Name != "Experiment" {
			continue
		}
		for node := range ast.Preorder(decl.Body) {
			clause, ok := node.(*ast.CaseClause)
			if !ok {
				continue
			}
			for _, expr := range clause.List {
				ident, ok := expr.(*ast.Ident)
				if !ok {
					continue
				}
				if value, ok := constants[ident.Name]; ok {
					displayNames[value] = true
				}
			}
		}
	}

	return displayNames, nil
}

func commentSentence(spec *ast.ValueSpec) string {
	if spec.Comment == nil {
		return ""
	}
	return sentence(spec.Comment.Text())
}

func render(route docgenenv.Route, known, safe codersdk.Experiments, descriptions map[string]string, displayNames map[string]bool) (string, error) {
	var b strings.Builder
	// The front matter title renders as the page heading, so the body starts
	// at the intro and its sections begin at level two.
	_, _ = b.WriteString(docgenenv.GeneratedHeader(route))
	_, _ = b.WriteString(intro)
	_, _ = b.WriteString(wildcardSection)

	if len(safe) > 0 {
		_, _ = b.WriteString("These experiments are safe to enable with the wildcard:\n\n")
		for _, exp := range safe {
			_, _ = fmt.Fprintf(&b, "- `%s`\n", exp)
		}
		_, _ = b.WriteString("\n")
	} else {
		_, _ = b.WriteString("No experiment currently carries that mark, so the wildcard enables nothing.\nEnable an experiment by naming its key.\n\n")
	}

	_, _ = b.WriteString(tableSection)
	_, _ = b.WriteString("| Experiment | Key | Description |\n|------------|-----|-------------|\n")

	known = slices.Clone(known)
	slices.Sort(known)
	for _, exp := range known {
		key := string(exp)
		desc, ok := descriptions[key]
		if !ok || desc == "" {
			return "", xerrors.Errorf("experiment %q has no description comment on its constant", key)
		}
		displayName := exp.DisplayName()
		if !displayNames[key] {
			return "", xerrors.Errorf("experiment %q has no explicit display name", key)
		}
		_, _ = fmt.Fprintf(&b, "| %s | `%s` | %s |\n", markdownCell(displayName), key, markdownCell(desc))
	}

	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

func markdownCell(value string) string {
	return strings.ReplaceAll(value, "|", `\|`)
}

// sentence collapses a comment to one line, capitalizes it, and gives it
// terminal punctuation so it reads as a sentence in a table cell.
func sentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return s
	}
	s = utilstrings.Capitalize(s)
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}
