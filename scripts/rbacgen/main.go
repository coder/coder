package main

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"go/format"
	"html/template"
	"log"
	"os"
	"slices"
	"strings"

	"github.com/coder/coder/v2/coderd/rbac/policy"
)

//go:embed object.gotmpl
var objectGoTpl string

// main will generate a file that lists all rbac objects.
// This is to provide an "AllResources" function that is always
// in sync.
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	out := generate(ctx)
	formatted, err := format.Source(out)
	if err != nil {
		log.Fatalf("Format template: %s", err.Error())
	}

	_, _ = fmt.Fprint(os.Stdout, string(formatted))
	return
}

func pascalCaseName(name string) string {
	names := strings.Split(name, "_")
	for i := range names {
		names[i] = capitalize(names[i])
	}
	return strings.Join(names, "")
}

func capitalize(name string) string {
	return strings.ToUpper(string(name[0])) + name[1:]
}

type Definition struct {
	policy.PermissionDefinition
	Type string
}

func (p Definition) FunctionName() string {
	if p.Name != "" {
		return p.Name
	}
	return p.Type
}

func generate(ctx context.Context) []byte {
	tpl, err := template.New("object.gotmpl").Funcs(template.FuncMap{
		"capitalize":     capitalize,
		"pascalCaseName": pascalCaseName,
	}).Parse(objectGoTpl)
	if err != nil {
		log.Fatalf("Failed to parse templates: %s", err.Error())
	}

	var out bytes.Buffer
	list := make([]Definition, 0)
	for t, v := range policy.RBACPermissions {
		v := v
		list = append(list, Definition{
			PermissionDefinition: v,
			Type:                 t,
		})
	}
	slices.SortFunc(list, func(a, b Definition) int {
		return strings.Compare(a.Type, b.Type)
	})

	err = tpl.Execute(&out, list)
	if err != nil {
		log.Fatalf("Execute template: %s", err.Error())
	}

	return out.Bytes()
}
