package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

const (
	baseDir = "./codersdk"
)

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var astFiles []*ast.File
	fset := token.NewFileSet()
	entries, err := os.ReadDir(baseDir)
	if err != nil {
		return err
	}

	for _, entry := range entries {
		goFile, err := parser.ParseFile(fset, filepath.Join(baseDir, entry.Name()), nil, 0)
		if err != nil {
			return err
		}

		astFiles = append(astFiles, goFile)
	}

	for _, astFile := range astFiles {
		for _, node := range astFile.Decls {
			switch node.(type) {
			case *ast.GenDecl:
				genDecl := node.(*ast.GenDecl)
				for _, spec := range genDecl.Specs {
					switch spec.(type) {
					case *ast.TypeSpec:
						typeSpec := spec.(*ast.TypeSpec)
						s, err := handleTypeSpec(typeSpec)
						if err != nil {
							break
						}

						fmt.Printf(s)
						break
					case *ast.ValueSpec:
						valueSpec := spec.(*ast.ValueSpec)
						s, err := handleValueSpec(valueSpec)
						if err != nil {
							break
						}

						fmt.Printf(s)
						break
					}
				}
			}
		}
	}

	return nil
}

func handleTypeSpec(typeSpec *ast.TypeSpec) (string, error) {
	jsonFields := 0
	s := ""
	switch typeSpec.Type.(type) {
	case *ast.StructType:
		s = fmt.Sprintf("export interface %s {\n", typeSpec.Name.Name)
		structType := typeSpec.Type.(*ast.StructType)
		for _, field := range structType.Fields.List {
			i, ok := field.Type.(*ast.Ident)
			if !ok {
				continue
			}
			fieldType, err := toTsType(i)
			if err != nil {
				continue
			}

			fieldName, err := toJSONField(field)
			if err != nil {
				continue
			}

			s = fmt.Sprintf("%s  %s: %s\n", s, fieldName, fieldType)
			jsonFields++
		}

		if jsonFields == 0 {
			return "", xerrors.New("no json fields")
		}

		return fmt.Sprintf("%s}\n\n", s), nil
	case *ast.Ident:
		ident := typeSpec.Type.(*ast.Ident)

		return fmt.Sprintf("type %s = %s\n\n", typeSpec.Name.Name, ident.Name), nil
	default:
		return "", xerrors.New("not struct or alias")
	}
}

func handleValueSpec(valueSpec *ast.ValueSpec) (string, error) {
	valueDecl := ""
	valueName := ""
	valueType := ""
	valueValue := ""
	for _, name := range valueSpec.Names {
		if name.Obj != nil && name.Obj.Kind == ast.Con {
			valueDecl = "const"
			valueName = name.Name
			break
		}
	}

	i, ok := valueSpec.Type.(*ast.Ident)
	if !ok {
		return "", xerrors.New("failed to cast type")
	}
	valueType = i.Name

	for _, value := range valueSpec.Values {
		bl, ok := value.(*ast.BasicLit)
		if !ok {
			return "", xerrors.New("failed to cast value")
		}
		valueValue = bl.Value
		break
	}

	return fmt.Sprintf("%s %s: %s = %s\n\n", valueDecl, valueName, valueType, valueValue), nil
}

func toTsType(e ast.Expr) (string, error) {
	i, ok := e.(*ast.Ident)
	if !ok {
		return "", xerrors.New("not ident")
	}
	fieldType := i.Name
	switch fieldType {
	case "bool":
		return "boolean", nil
	case "uint64", "uint32", "float64":
		return "number", nil
	}

	return fieldType, nil
}

func toJSONField(field *ast.Field) (string, error) {
	if field.Tag != nil && field.Tag.Value != "" {
		fieldName := strings.Trim(field.Tag.Value, "`")
		for _, pair := range strings.Split(fieldName, " ") {
			if strings.Contains(pair, `json:`) {
				fieldName := strings.TrimPrefix(pair, `json:`)
				fieldName = strings.Trim(fieldName, `"`)
				fieldName = strings.Split(fieldName, ",")[0]

				return fieldName, nil
			}
		}
	}

	return "", xerrors.New("no json tag")
}
