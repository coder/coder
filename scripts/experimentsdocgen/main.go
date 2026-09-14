// Command experimentsdocgen generates the experiments reference at
// docs/reference/experiments.md from codersdk.
//
// Two sources are combined. The set of experiments, their display names, and
// whether each one is safe to enable through the wildcard come from the
// codersdk package at run time. The per-experiment descriptions live only as
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
coder server --experiments=example,workspace-usage
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

	content, err := render(*route, descriptions)
	if err != nil {
		flog.Fatalf("render experiments reference: %v", err)
	}
	if err := atomicwrite.File(*out, []byte(content)); err != nil {
		flog.Fatalf("write %s: %v", *out, err)
	}
	flog.Successf("wrote %s", *out)
}

// readDescriptions returns the trailing comment on each Experiment constant,
// keyed by the experiment's string value. The comments are the only
// description the codebase carries for an experiment, and they are not
// reachable at run time.
func readDescriptions(path string) (map[string]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, xerrors.Errorf("parse %q: %w", path, err)
	}

	descriptions := map[string]string{}
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		ident, ok := spec.Type.(*ast.Ident)
		if !ok || ident.Name != "Experiment" || len(spec.Values) != 1 {
			return true
		}
		lit, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		value, err := strconv.Unquote(lit.Value)
		if err != nil {
			return true
		}
		descriptions[value] = commentText(spec)
		return true
	})

	if len(descriptions) == 0 {
		return nil, xerrors.Errorf("no Experiment constants found in %q", path)
	}
	return descriptions, nil
}

// commentText prefers the trailing comment on the constant's own line and
// falls back to the doc comment above it.
func commentText(spec *ast.ValueSpec) string {
	if spec.Comment != nil {
		return sentence(spec.Comment.Text())
	}
	if spec.Doc != nil {
		return sentence(spec.Doc.Text())
	}
	return ""
}

func render(route docgenenv.Route, descriptions map[string]string) (string, error) {
	var b strings.Builder
	// The front matter title renders as the page heading, so the body starts
	// at the intro and its sections begin at level two.
	_, _ = b.WriteString(docgenenv.GeneratedHeader(route))
	_, _ = b.WriteString(intro)
	_, _ = b.WriteString(wildcardSection)

	if len(codersdk.ExperimentsSafe) > 0 {
		_, _ = b.WriteString("These experiments are safe to enable with the wildcard:\n\n")
		for _, exp := range codersdk.ExperimentsSafe {
			_, _ = fmt.Fprintf(&b, "- `%s`\n", exp)
		}
		_, _ = b.WriteString("\n")
	} else {
		_, _ = b.WriteString("No experiment currently carries that mark, so the wildcard enables nothing.\nEnable an experiment by naming its key.\n\n")
	}

	_, _ = b.WriteString(tableSection)
	_, _ = b.WriteString("| Experiment | Key | Description |\n|------------|-----|-------------|\n")

	known := slices.Clone(codersdk.ExperimentsKnown)
	slices.Sort(known)
	for _, exp := range known {
		key := string(exp)
		desc, ok := descriptions[key]
		if !ok {
			return "", xerrors.Errorf("experiment %q has no description comment on its constant", key)
		}
		_, _ = fmt.Fprintf(&b, "| %s | `%s` | %s |\n", exp.DisplayName(), key, desc)
	}

	return strings.TrimRight(b.String(), "\n") + "\n", nil
}

// sentence collapses a comment to one line, capitalizes it, and gives it
// terminal punctuation so it reads as a sentence in a table cell.
func sentence(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if s == "" {
		return s
	}
	s = strings.ToUpper(s[:1]) + s[1:]
	if !strings.HasSuffix(s, ".") {
		s += "."
	}
	return s
}
