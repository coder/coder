// Package main audits description coverage in coderd/apidoc/swagger.json.
//
// It is a review aid for the series of PRs that fill in REST API
// descriptions. It is not used in CI.
//
// For each API tag, counting only operations that the docs engine publishes
// (those without x-apidocgen.skip), it reports:
//   - operations with no description
//   - parameters, and "thin" parameters (a description of 3 words or fewer)
//   - codersdk schema fields with no description
//
// Each codersdk schema is counted once, under the first tag in PR order whose
// published operations reach it, directly or through $ref chains.
//
// Usage:
//
//	go run ./scripts/apidocgen/apiaudit
//	go run ./scripts/apidocgen/apiaudit -tag Users
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/xerrors"
)

const (
	defaultSwagger = "coderd/apidoc/swagger.json"
	defaultSDKDir  = "codersdk"
	defaultOrder   = "Users,Authorization,Organizations,Members,Builds,Workspaces,Templates"

	untaggedName   = "(untagged)"
	definitionsRef = "#/definitions/"
	sdkPrefix      = "codersdk."
	unmappedFile   = "?"
	thinWordLimit  = 3
	maxFilesShown  = 3
	maxUnmappedOut = 10
)

// httpMethods lists the operation keys of a swagger path item.
var httpMethods = []string{"get", "post", "put", "patch", "delete", "head", "options"}

type swaggerDoc struct {
	Tags        []struct{ Description string }        `json:"tags"`
	Paths       map[string]map[string]json.RawMessage `json:"paths"`
	Definitions map[string]map[string]any             `json:"definitions"`
}

type swaggerOperation struct {
	Tags        []string `json:"tags"`
	Summary     string   `json:"summary"`
	Description string   `json:"description"`
	Parameters  []struct {
		Name        string `json:"name"`
		In          string `json:"in"`
		Description string `json:"description"`
	} `json:"parameters"`
	APIDocGen struct {
		Skip bool `json:"skip"`
	} `json:"x-apidocgen"`
}

type operation struct {
	tag         string
	method      string
	path        string
	summary     string
	description string
	skip        bool
	params      []parameter
	schemas     map[string]struct{}
}

type parameter struct {
	name        string
	in          string
	description string
}

func (p parameter) thin() bool {
	return len(strings.Fields(p.description)) <= thinWordLimit
}

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "apiaudit: %v\n", err)
		os.Exit(1)
	}
}

// run parses args and writes the report to stdout.
func run(args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("apiaudit", flag.ContinueOnError)
	swaggerPath := fs.String("swagger", defaultSwagger, "Path to swagger.json.")
	sdkDir := fs.String("codersdk", defaultSDKDir, "Path to the codersdk directory.")
	orderArg := fs.String("order", defaultOrder, "Comma-separated tag order that decides which tag owns a shared schema. Remaining tags follow by published operation count.")
	tag := fs.String("tag", "", "Print what is left to fix for this one tag instead of the summary table.")
	if err := fs.Parse(args); err != nil {
		return xerrors.Errorf("parse flags: %w", err)
	}

	a, err := load(*swaggerPath, *sdkDir, strings.Split(*orderArg, ","))
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if *tag != "" {
		if err := a.writeTagDetail(&buf, *tag); err != nil {
			return err
		}
	} else {
		a.writeSummary(&buf)
	}
	if _, err := stdout.Write(buf.Bytes()); err != nil {
		return xerrors.Errorf("write output: %w", err)
	}
	return nil
}

type audit struct {
	doc   swaggerDoc
	ops   []operation
	byTag map[string][]int
	order []string
	owner map[string]string
	// ownerOrder lists schemas in the order they were first claimed.
	ownerOrder []string
	typeFile   map[string]string
}

func load(swaggerPath, sdkDir string, priority []string) (*audit, error) {
	raw, err := os.ReadFile(swaggerPath)
	if err != nil {
		return nil, xerrors.Errorf("read swagger: %w", err)
	}
	a := &audit{byTag: map[string][]int{}, owner: map[string]string{}}
	if err := json.Unmarshal(raw, &a.doc); err != nil {
		return nil, xerrors.Errorf("parse swagger %q: %w", swaggerPath, err)
	}
	a.typeFile, err = mapTypesToFiles(sdkDir)
	if err != nil {
		return nil, err
	}
	if err := a.collectOps(); err != nil {
		return nil, err
	}
	a.computeOrder(priority)
	a.computeOwners()
	return a, nil
}

func (a *audit) collectOps() error {
	paths := sortedKeys(a.doc.Paths)
	for _, path := range paths {
		item := a.doc.Paths[path]
		for _, method := range httpMethods {
			rawOp, ok := item[method]
			if !ok {
				continue
			}
			var so swaggerOperation
			if err := json.Unmarshal(rawOp, &so); err != nil {
				return xerrors.Errorf("parse operation %s %s: %w", strings.ToUpper(method), path, err)
			}
			var generic struct {
				Parameters any `json:"parameters"`
				Responses  any `json:"responses"`
			}
			if err := json.Unmarshal(rawOp, &generic); err != nil {
				return xerrors.Errorf("parse operation refs %s %s: %w", strings.ToUpper(method), path, err)
			}
			direct := map[string]struct{}{}
			collectRefs(generic.Parameters, direct)
			collectRefs(generic.Responses, direct)

			op := operation{
				tag:         untaggedName,
				method:      strings.ToUpper(method),
				path:        path,
				summary:     so.Summary,
				description: so.Description,
				skip:        so.APIDocGen.Skip,
				schemas:     a.closure(direct),
			}
			if len(so.Tags) > 0 {
				op.tag = so.Tags[0]
			}
			for _, p := range so.Parameters {
				op.params = append(op.params, parameter{name: p.Name, in: p.In, description: p.Description})
			}
			a.byTag[op.tag] = append(a.byTag[op.tag], len(a.ops))
			a.ops = append(a.ops, op)
		}
	}
	return nil
}

// collectRefs adds every #/definitions/ reference found under node to out.
func collectRefs(node any, out map[string]struct{}) {
	switch n := node.(type) {
	case map[string]any:
		if r, ok := n["$ref"].(string); ok && strings.HasPrefix(r, definitionsRef) {
			out[strings.TrimPrefix(r, definitionsRef)] = struct{}{}
		}
		for _, v := range n {
			collectRefs(v, out)
		}
	case []any:
		for _, v := range n {
			collectRefs(v, out)
		}
	}
}

// closure returns names plus every definition reachable from them.
func (a *audit) closure(names map[string]struct{}) map[string]struct{} {
	seen := map[string]struct{}{}
	stack := make([]string, 0, len(names))
	for n := range names {
		stack = append(stack, n)
	}
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if _, ok := seen[n]; ok {
			continue
		}
		def, ok := a.doc.Definitions[n]
		if !ok {
			continue
		}
		seen[n] = struct{}{}
		next := map[string]struct{}{}
		collectRefs(def, next)
		for r := range next {
			if _, ok := seen[r]; !ok {
				stack = append(stack, r)
			}
		}
	}
	return seen
}

func (a *audit) publishedCount(tag string) int {
	n := 0
	for _, i := range a.byTag[tag] {
		if !a.ops[i].skip {
			n++
		}
	}
	return n
}

// computeOrder puts the requested tags first, then the rest by published
// operation count descending, ties broken by name.
func (a *audit) computeOrder(priority []string) {
	inOrder := map[string]struct{}{}
	for _, t := range priority {
		t = strings.TrimSpace(t)
		if _, ok := a.byTag[t]; !ok {
			continue
		}
		if _, dup := inOrder[t]; dup {
			continue
		}
		inOrder[t] = struct{}{}
		a.order = append(a.order, t)
	}
	var rest []string
	for t := range a.byTag {
		if _, ok := inOrder[t]; !ok {
			rest = append(rest, t)
		}
	}
	sort.Slice(rest, func(i, j int) bool {
		ci, cj := a.publishedCount(rest[i]), a.publishedCount(rest[j])
		if ci != cj {
			return ci > cj
		}
		return rest[i] < rest[j]
	})
	a.order = append(a.order, rest...)
}

// computeOwners assigns each schema to the first tag in order whose published
// operations reach it.
func (a *audit) computeOwners() {
	for _, t := range a.order {
		for _, i := range a.byTag[t] {
			if a.ops[i].skip {
				continue
			}
			for _, s := range sortedKeys(a.ops[i].schemas) {
				if _, ok := a.owner[s]; !ok {
					a.owner[s] = t
					a.ownerOrder = append(a.ownerOrder, s)
				}
			}
		}
	}
}

func blank(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) == ""
}

// fieldCounts returns the property count of a definition and the sorted names
// of properties with no description.
func (a *audit) fieldCounts(name string) (total int, undocumented []string) {
	props, _ := a.doc.Definitions[name]["properties"].(map[string]any)
	for _, k := range sortedKeys(props) {
		total++
		p, _ := props[k].(map[string]any)
		if blank(p["description"]) {
			undocumented = append(undocumented, k)
		}
	}
	return total, undocumented
}

func (a *audit) ownedSDKSchemas(tag string) []string {
	var owned []string
	for _, s := range a.ownerOrder {
		if a.owner[s] == tag && strings.HasPrefix(s, sdkPrefix) {
			owned = append(owned, s)
		}
	}
	return owned
}

func (a *audit) fileFor(schema string) string {
	if f, ok := a.typeFile[strings.TrimPrefix(schema, sdkPrefix)]; ok {
		return f
	}
	return unmappedFile
}

type totals struct {
	pub, noDesc, params, thin, fieldsMissing, fieldsTotal, skipped int
}

func (a *audit) writeSummary(w *bytes.Buffer) {
	_, _ = fmt.Fprintln(w, "| Tag | Published ops | No description | Params | Thin params (3 words or fewer) | codersdk fields with no comment | Main codersdk files | Skipped ops |")
	_, _ = fmt.Fprintln(w, "|---|---:|---:|---:|---:|---:|---|---:|")
	var t totals
	for _, tag := range a.order {
		var row totals
		for _, i := range a.byTag[tag] {
			op := a.ops[i]
			if op.skip {
				row.skipped++
				continue
			}
			row.pub++
			if blank(op.description) {
				row.noDesc++
			}
			row.params += len(op.params)
			for _, p := range op.params {
				if p.thin() {
					row.thin++
				}
			}
		}
		files := map[string]int{}
		var fileOrder []string
		for _, s := range a.ownedSDKSchemas(tag) {
			total, missing := a.fieldCounts(s)
			row.fieldsTotal += total
			row.fieldsMissing += len(missing)
			f := a.fileFor(s)
			if _, seen := files[f]; !seen {
				fileOrder = append(fileOrder, f)
			}
			files[f] += len(missing)
		}
		_, _ = fmt.Fprintf(w, "| %s | %d | %d | %d | %d | %d of %d | %s | %d |\n",
			tag, row.pub, row.noDesc, row.params, row.thin, row.fieldsMissing, row.fieldsTotal, topFiles(files, fileOrder), row.skipped)
		t.pub += row.pub
		t.noDesc += row.noDesc
		t.params += row.params
		t.thin += row.thin
		t.fieldsMissing += row.fieldsMissing
		t.fieldsTotal += row.fieldsTotal
		t.skipped += row.skipped
	}
	_, _ = fmt.Fprintf(w, "| **Total** | **%d** | **%d** | **%d** | **%d** | **%d of %d** | | **%d** |\n",
		t.pub, t.noDesc, t.params, t.thin, t.fieldsMissing, t.fieldsTotal, t.skipped)

	var sdk, skipOnly, other []string
	for name := range a.doc.Definitions {
		switch {
		case !strings.HasPrefix(name, sdkPrefix):
			other = append(other, name)
		default:
			sdk = append(sdk, name)
			if _, ok := a.owner[name]; !ok {
				skipOnly = append(skipOnly, name)
			}
		}
	}
	sort.Strings(sdk)
	sort.Strings(skipOnly)

	sum := func(names []string) (fields, missing int) {
		for _, n := range names {
			f, m := a.fieldCounts(n)
			fields += f
			missing += len(m)
		}
		return fields, missing
	}
	sdkFields, sdkMissing := sum(sdk)
	skipFields, skipMissing := sum(skipOnly)
	otherFields, otherMissing := sum(other)
	typeDesc := 0
	for _, n := range sdk {
		if !blank(a.doc.Definitions[n]["description"]) {
			typeDesc++
		}
	}
	skippedWithDesc := 0
	for _, op := range a.ops {
		if op.skip && !blank(op.description) {
			skippedWithDesc++
		}
	}
	tagDesc := 0
	for _, td := range a.doc.Tags {
		if strings.TrimSpace(td.Description) != "" {
			tagDesc++
		}
	}
	var unmapped []string
	for _, n := range sdk {
		if _, ok := a.typeFile[strings.TrimPrefix(n, sdkPrefix)]; !ok {
			unmapped = append(unmapped, n)
		}
	}

	_, _ = fmt.Fprintln(w)
	_, _ = fmt.Fprintf(w, "codersdk schemas: %d, fields %d, no comment %d\n", len(sdk), sdkFields, sdkMissing)
	_, _ = fmt.Fprintf(w, "codersdk schemas reached only by skipped ops: %d, no comment %d of %d\n", len(skipOnly), skipMissing, skipFields)
	_, _ = fmt.Fprintf(w, "codersdk schemas with a type-level description: %d\n", typeDesc)
	_, _ = fmt.Fprintf(w, "other schemas: %d, no comment %d of %d\n", len(other), otherMissing, otherFields)
	_, _ = fmt.Fprintf(w, "all ops %d, published %d, published with description %d, skipped with description %d\n",
		len(a.ops), t.pub, t.pub-t.noDesc, skippedWithDesc)
	_, _ = fmt.Fprintf(w, "tags with a tag-level description: %d of %d\n", tagDesc, len(a.byTag))
	shown := unmapped
	if len(shown) > maxUnmappedOut {
		shown = shown[:maxUnmappedOut]
	}
	quoted := make([]string, len(shown))
	for i, s := range shown {
		quoted[i] = "'" + s + "'"
	}
	_, _ = fmt.Fprintf(w, "unmapped codersdk types: %d [%s]\n", len(unmapped), strings.Join(quoted, ", "))
}

// topFiles formats the files with the most undocumented fields, skipping files
// with none. Ties keep the order in which files were first seen (order), so the
// result is deterministic and matches the original audit script.
func topFiles(files map[string]int, order []string) string {
	names := make([]string, 0, len(order))
	for _, f := range order {
		if files[f] > 0 {
			names = append(names, f)
		}
	}
	sort.SliceStable(names, func(i, j int) bool {
		return files[names[i]] > files[names[j]]
	})
	if len(names) > maxFilesShown {
		names = names[:maxFilesShown]
	}
	for i, n := range names {
		names[i] = "`" + n + "`"
	}
	return strings.Join(names, ", ")
}

// writeTagDetail lists what is left to fix for one tag.
func (a *audit) writeTagDetail(w *bytes.Buffer, tag string) error {
	if _, ok := a.byTag[tag]; !ok {
		return xerrors.Errorf("unknown tag %q, known tags: %s", tag, strings.Join(a.order, ", "))
	}
	var noDesc, thin []string
	for _, i := range a.byTag[tag] {
		op := a.ops[i]
		if op.skip {
			continue
		}
		if blank(op.description) {
			noDesc = append(noDesc, fmt.Sprintf("%s %s  %s", op.method, op.path, op.summary))
		}
		for _, p := range op.params {
			if p.thin() {
				thin = append(thin, fmt.Sprintf("%s %s  %s (%s): %q", op.method, op.path, p.name, p.in, p.description))
			}
		}
	}
	var fields []string
	for _, s := range a.ownedSDKSchemas(tag) {
		_, missing := a.fieldCounts(s)
		for _, f := range missing {
			fields = append(fields, fmt.Sprintf("%s.%s  %s", s, f, a.fileFor(s)))
		}
	}
	sort.Strings(noDesc)
	sort.Strings(thin)
	sort.Strings(fields)

	section := func(title string, lines []string) {
		_, _ = fmt.Fprintf(w, "## %s (%d)\n", title, len(lines))
		for _, l := range lines {
			_, _ = fmt.Fprintln(w, l)
		}
	}
	section("Operations with no description", noDesc)
	_, _ = fmt.Fprintln(w)
	section("Thin parameters", thin)
	_, _ = fmt.Fprintln(w)
	section("codersdk fields with no comment", fields)
	return nil
}

// mapTypesToFiles maps each top-level type name declared in the non-test Go
// files of dir to the base name of its file. The first file in name order wins.
func mapTypesToFiles(dir string) (map[string]string, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return nil, xerrors.Errorf("glob %q: %w", dir, err)
	}
	sort.Strings(matches)
	out := map[string]string{}
	fset := token.NewFileSet()
	for _, path := range matches {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
		if err != nil {
			return nil, xerrors.Errorf("parse %q: %w", path, err)
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				if _, exists := out[ts.Name.Name]; !exists {
					out[ts.Name.Name] = filepath.Base(path)
				}
			}
		}
	}
	return out, nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
