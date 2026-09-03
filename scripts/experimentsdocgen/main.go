// experimentsdocgen writes docs/experiments.json: every user-facing experiment
// this version of Coder knows about (codersdk.ExperimentsKnown minus the
// placeholders in notDocumented), its display name,
// the description from the constant's comment in codersdk/deployment.go, and
// whether `--experiments=*` enables it (membership in codersdk.ExperimentsSafe).
//
// The documentation reads the file to list experiments per version and to
// label experimental content, so the list is generated from the code rather
// than maintained by hand. `make gen` runs it; CI fails on a stale copy.
package main

import (
	"encoding/json"
	"flag"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/scripts/atomicwrite"
)

// experimentsDoc is the on-disk shape of docs/experiments.json.
type experimentsDoc struct {
	SchemaVersion int               `json:"schemaVersion"`
	Experiments   []experimentEntry `json:"experiments"`
}

type experimentEntry struct {
	// ID is the flag value passed to --experiments.
	ID string `json:"id"`
	// DisplayName is codersdk.Experiment.DisplayName() for the flag.
	DisplayName string `json:"displayName"`
	// Description is the trailing comment on the constant, or empty.
	Description string `json:"description"`
	// Safe reports whether `--experiments=*` enables the flag.
	Safe bool `json:"safe"`
}

func main() {
	var (
		source string
		out    string
		dryRun bool
	)
	flag.StringVar(&source, "source", "codersdk/deployment.go", "Go file declaring the Experiment constants")
	flag.StringVar(&out, "out", "docs/experiments.json", "Path of the JSON file to write")
	flag.BoolVar(&dryRun, "dry-run", false, "Print the JSON instead of writing it")
	flag.Parse()

	src, err := os.ReadFile(source)
	if err != nil {
		log.Fatalf("read %s: %v", source, err)
	}
	descriptions, err := parseExperimentDescriptions(src)
	if err != nil {
		log.Fatalf("parse %s: %v", source, err)
	}

	doc := buildExperimentsDoc(documented(codersdk.ExperimentsKnown), codersdk.ExperimentsSafe, descriptions, codersdk.Experiment.DisplayName)
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		log.Fatalf("encode: %v", err)
	}
	data = append(data, '\n')

	if dryRun {
		_, _ = os.Stdout.Write(data)
		return
	}
	if err := atomicwrite.File(out, data); err != nil {
		log.Fatalf("write %s: %v", out, err)
	}
}

// notDocumented lists experiments that exist for the code's own purposes and
// never describe a user-facing feature, so the documentation omits them.
var notDocumented = map[codersdk.Experiment]bool{
	codersdk.ExperimentExample: true, // A placeholder kept for tests.
}

// documented filters `known` down to the experiments the docs should list.
func documented(known codersdk.Experiments) codersdk.Experiments {
	out := make(codersdk.Experiments, 0, len(known))
	for _, experiment := range known {
		if !notDocumented[experiment] {
			out = append(out, experiment)
		}
	}
	return out
}

// parseExperimentDescriptions maps each `Experiment` constant's string value to
// the comment beside it (`ExperimentFoo Experiment = "foo" // Enables foo.`),
// falling back to a doc comment above the constant. Constants of any other
// type are ignored, so the file may declare whatever else it likes.
func parseExperimentDescriptions(src []byte) (map[string]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "deployment.go", src, parser.ParseComments)
	if err != nil {
		return nil, xerrors.Errorf("parse: %w", err)
	}

	descriptions := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || len(value.Values) != 1 {
				continue
			}
			typeName, ok := value.Type.(*ast.Ident)
			if !ok || typeName.Name != "Experiment" {
				continue
			}
			lit, ok := value.Values[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			id, err := strconv.Unquote(lit.Value)
			if err != nil {
				return nil, xerrors.Errorf("unquote %s: %w", lit.Value, err)
			}
			description := ""
			switch {
			case value.Comment != nil:
				description = value.Comment.Text()
			case value.Doc != nil:
				description = value.Doc.Text()
			}
			descriptions[id] = strings.TrimSpace(description)
		}
	}
	return descriptions, nil
}

// buildExperimentsDoc assembles the document for `known`, sorted by id, marking
// each entry safe when it also appears in `safe`. Descriptions missing from the
// map are left empty rather than failing: a flag with no comment is still a flag.
func buildExperimentsDoc(
	known codersdk.Experiments,
	safe codersdk.Experiments,
	descriptions map[string]string,
	displayName func(codersdk.Experiment) string,
) experimentsDoc {
	safeSet := make(map[codersdk.Experiment]bool, len(safe))
	for _, experiment := range safe {
		safeSet[experiment] = true
	}
	entries := make([]experimentEntry, 0, len(known))
	for _, experiment := range known {
		entries = append(entries, experimentEntry{
			ID:          string(experiment),
			DisplayName: displayName(experiment),
			Description: descriptions[string(experiment)],
			Safe:        safeSet[experiment],
		})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return experimentsDoc{SchemaVersion: 1, Experiments: entries}
}
