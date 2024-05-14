package main

import (
	"bytes"
	"context"
	_ "embed"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
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

	out, err := generate(ctx)
	if err != nil {
		log.Fatalf("Generate source: %s", err.Error())
	}

	formatted, err := format.Source(out)
	if err != nil {
		log.Fatalf("Format template: %s", err.Error())
	}

	_, _ = fmt.Fprint(os.Stdout, string(formatted))
	return
}

func pascalCaseName[T ~string](name T) string {
	names := strings.Split(string(name), "_")
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

func fileActions(file *ast.File) map[string]string {
	// actions is a map from the enum value -> enum name
	actions := make(map[string]string)

	// Find the action consts
fileDeclLoop:
	for _, decl := range file.Decls {
		switch typedDecl := decl.(type) {
		case *ast.GenDecl:
			if len(typedDecl.Specs) == 0 {
				continue
			}
			// This is the right on, loop over all idents, pull the actions
			for _, spec := range typedDecl.Specs {
				vSpec, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue fileDeclLoop
				}

				typeIdent, ok := vSpec.Type.(*ast.Ident)
				if !ok {
					continue fileDeclLoop
				}

				if typeIdent.Name != "Action" || len(vSpec.Values) != 1 || len(vSpec.Names) != 1 {
					continue fileDeclLoop
				}

				literal, ok := vSpec.Values[0].(*ast.BasicLit)
				if !ok {
					continue fileDeclLoop
				}
				actions[strings.Trim(literal.Value, `"`)] = vSpec.Names[0].Name
			}
		default:
			continue
		}
	}
	return actions
}

func generate(ctx context.Context) ([]byte, error) {
	// Parse the policy.go file for the action enums
	f, err := parser.ParseFile(token.NewFileSet(), "./coderd/rbac/policy/policy.go", nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing policy.go: %w", err)
	}
	actionMap := fileActions(f)

	var errorList []error
	var x int
	tpl, err := template.New("object.gotmpl").Funcs(template.FuncMap{
		"capitalize":     capitalize,
		"pascalCaseName": pascalCaseName[string],
		"actionsList": func() []string {
			tmp := make([]string, 0)
			for _, actionEnum := range actionMap {
				tmp = append(tmp, actionEnum)
			}
			return tmp
		},
		"actionEnum": func(action policy.Action) string {
			x++
			v, ok := actionMap[string(action)]
			if !ok {
				errorList = append(errorList, fmt.Errorf("action value %q does not have a constant a matching enum constant", action))
			}
			return v
		},
		"concat": func(strs ...string) string { return strings.Join(strs, "") },
	}).Parse(objectGoTpl)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
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
		return nil, fmt.Errorf("execute template: %w", err)
	}

	if len(errorList) > 0 {
		return nil, errors.Join(errorList...)
	}

	return out.Bytes(), nil
}
