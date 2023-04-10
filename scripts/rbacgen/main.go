package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"go/format"
	"go/types"
	"html/template"
	"log"
	"os"
	"sort"

	"golang.org/x/tools/go/packages"
)

//go:embed object.gotmpl
var objectGoTpl string

type TplState struct {
	ResourceNames []string
}

// main will generate a file that lists all rbac objects.
// This is to provide an "AllResources" function that is always
// in sync.
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	path := "."
	if len(os.Args) > 1 {
		path = os.Args[1]
	}

	cfg := &packages.Config{
		Mode:    packages.NeedTypes | packages.NeedName | packages.NeedTypesInfo | packages.NeedDeps,
		Tests:   false,
		Context: ctx,
	}

	pkgs, err := packages.Load(cfg, path)
	if err != nil {
		log.Fatalf("Failed to load package: %s", err.Error())
	}

	if len(pkgs) != 1 {
		log.Fatalf("Expected 1 package, got %d", len(pkgs))
	}

	rbacPkg := pkgs[0]
	if rbacPkg.Name != "rbac" {
		log.Fatalf("Expected rbac package, got %q", rbacPkg.Name)
	}

	tpl, err := template.New("object.gotmpl").Parse(objectGoTpl)
	if err != nil {
		log.Fatalf("Failed to parse templates: %s", err.Error())
	}

	var out bytes.Buffer
	err = tpl.Execute(&out, TplState{
		ResourceNames: allResources(rbacPkg),
	})

	if err != nil {
		log.Fatalf("Execute template: %s", err.Error())
	}

	formatted, err := format.Source(out.Bytes())
	if err != nil {
		log.Fatalf("Format template: %s", err.Error())
	}

	_, _ = fmt.Fprint(os.Stdout, string(formatted))
}

func allResources(pkg *packages.Package) []string {
	var resources []string
	names := pkg.Types.Scope().Names()
	for _, name := range names {
		obj, ok := pkg.Types.Scope().Lookup(name).(*types.Var)
		if ok && obj.Type().String() == "github.com/coder/coder/coderd/rbac.Object" {
			resources = append(resources, obj.Name())
		}
	}
	sort.Strings(resources)
	return resources
}
