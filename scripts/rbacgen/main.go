package main

import (
	"bytes"
	_ "embed"
	"errors"
	"flag"
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

//go:embed rbacobject.gotmpl
var rbacObjectTemplate string

//go:embed codersdk.gotmpl
var codersdkTemplate string

func usage() {
	_, _ = fmt.Println("Usage: rbacgen <codersdk|rbac>")
	_, _ = fmt.Println("Must choose a template target.")
}

// main will generate a file that lists all rbac objects.
// This is to provide an "AllResources" function that is always
// in sync.
func main() {
	flag.Parse()

	if len(flag.Args()) < 1 {
		usage()
		os.Exit(1)
	}

	// It did not make sense to have 2 different generators that do essentially
	// the same thing, but different format for the BE and the sdk.
	// So the argument switches the go template to use.
	var source string
	switch strings.ToLower(flag.Args()[0]) {
	case "codersdk":
		source = codersdkTemplate
	case "rbac":
		source = rbacObjectTemplate
	default:
		_, _ = fmt.Fprintf(os.Stderr, "%q is not a valid templte target\n", flag.Args()[0])
		usage()
		os.Exit(2)
	}

	out, err := generateRbacObjects(source)
	if err != nil {
		log.Fatalf("Generate source: %s", err.Error())
	}

	formatted, err := format.Source(out)
	if err != nil {
		log.Fatalf("Format template: %s", err.Error())
	}

	_, _ = fmt.Fprint(os.Stdout, string(formatted))
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

// fileActions is required because we cannot get the variable name of the enum
// at runtime. So parse the package to get it. This is purely to ensure enum
// names are consistent, which is a bit annoying, but not too bad.
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

type ActionDetails struct {
	Enum  string
	Value string
}

// generateRbacObjects will take the policy.go file, and send it as input
// to the go templates. Some AST of the Action enum is also included.
func generateRbacObjects(templateSource string) ([]byte, error) {
	// Parse the policy.go file for the action enums
	f, err := parser.ParseFile(token.NewFileSet(), "./coderd/rbac/policy/policy.go", nil, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parsing policy.go: %w", err)
	}
	actionMap := fileActions(f)
	actionList := make([]ActionDetails, 0)
	for value, enum := range actionMap {
		actionList = append(actionList, ActionDetails{
			Enum:  enum,
			Value: value,
		})
	}

	// Sorting actions for auto gen consistency.
	slices.SortFunc(actionList, func(a, b ActionDetails) int {
		return strings.Compare(a.Enum, b.Enum)
	})

	var errorList []error
	var x int
	tpl, err := template.New("object.gotmpl").Funcs(template.FuncMap{
		"capitalize":     capitalize,
		"pascalCaseName": pascalCaseName[string],
		"actionsList": func() []ActionDetails {
			return actionList
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
	}).Parse(templateSource)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}

	// Convert to sorted list for autogen consistency.
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
