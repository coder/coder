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

		for _, node := range goFile.Decls {
			switch node.(type) {

			case *ast.GenDecl:
				genDecl := node.(*ast.GenDecl)
				for _, spec := range genDecl.Specs {
					switch spec.(type) {
					case *ast.TypeSpec:
						typeSpec := spec.(*ast.TypeSpec)
						s, err := writeStruct(typeSpec)
						if err != nil {
							continue
						}

						fmt.Printf(s)
					}
				}
			}
		}
	}

	return nil
}

func writeStruct(typeSpec *ast.TypeSpec) (string, error) {
	s := fmt.Sprintf("export interface %s {\n", typeSpec.Name.Name)
	jsonFields := 0
	switch typeSpec.Type.(type) {
	case *ast.StructType:
		structType := typeSpec.Type.(*ast.StructType)
		for _, field := range structType.Fields.List {
			i, ok := field.Type.(*ast.Ident)
			if !ok {
				continue
			}
			fieldType := i.Name
			switch fieldType {
			case "bool":
				fieldType = "boolean"
			case "uint64", "uint32", "float64":
				fieldType = "number"
			}

			fieldName := ""
			if field.Tag != nil && field.Tag.Value != "" {
				for _, pair := range strings.Split(field.Tag.Value, " ") {
					if strings.HasPrefix(pair, "`json:\"") {
						fieldName = strings.TrimPrefix(pair, "`json:\"")
						fieldName = strings.Split(fieldName, ",")[0]
						fieldName = strings.TrimSuffix(fieldName, "`")
						fieldName = strings.TrimSuffix(fieldName, "\"")
						break
					}
				}
			}
			if fieldName == "" {
				break
			}

			s = fmt.Sprintf("%s  %s: %s\n", s, fieldName, fieldType)
			jsonFields++
		}
	default:
		return "", xerrors.New("not struct")
	}

	if jsonFields == 0 {
		return "", xerrors.New("no json fields")
	}

	return fmt.Sprintf("%s}\n\n", s), nil
}
