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
		return xerrors.Errorf("reading dir %s: %w", baseDir, err)
	}

	// loop each file in directory
	for _, entry := range entries {
		astFile, err := parser.ParseFile(fset, filepath.Join(baseDir, entry.Name()), nil, 0)
		if err != nil {
			return xerrors.Errorf("parsing file %s: %w", filepath.Join(baseDir, entry.Name()), err)
		}

		// loop each declaration in file
		for _, node := range astFile.Decls {
			genDecl, ok := node.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range genDecl.Specs {
				pos := fset.Position(spec.Pos())
				switch s := spec.(type) {
				// TypeSpec case for structs and type alias
				case *ast.TypeSpec:
					out, err := handleTypeSpec(s, pos)
					if err != nil {
						break
					}

					_, _ = fmt.Printf(out)
				// ValueSpec case for const "enums"
				case *ast.ValueSpec:
					out, err := handleValueSpec(s, pos)
					if err != nil {
						break
					}

					_, _ = fmt.Printf(out)
				}
			}
		}
	}

	return nil
}

func handleTypeSpec(typeSpec *ast.TypeSpec, pos token.Position) (string, error) {
	jsonFields := 0
	s := fmt.Sprintf("// From %s.\n", pos.String())
	switch t := typeSpec.Type.(type) {
	// Struct declaration
	case *ast.StructType:
		s = fmt.Sprintf("%sexport interface %s {\n", s, typeSpec.Name.Name)
		for _, field := range t.Fields.List {
			i, optional, err := getIdent(field.Type)
			if err != nil {
				continue
			}

			fieldType := toTsType(i.Name)
			if fieldType == "" {
				continue
			}

			fieldName := toJSONField(field)
			if fieldType == "" {
				continue
			}

			s = fmt.Sprintf("%s  %s%s: %s\n", s, fieldName, optional, fieldType)
			jsonFields++
		}

		// Do not print struct if it has no json fields
		if jsonFields == 0 {
			return "", xerrors.New("no json fields")
		}

		return fmt.Sprintf("%s}\n\n", s), nil
	// Type alias declaration
	case *ast.Ident:
		return fmt.Sprintf("%stype %s = %s\n\n", s, typeSpec.Name.Name, t.Name), nil
	default:
		return "", xerrors.New("not struct or alias")
	}
}

func handleValueSpec(valueSpec *ast.ValueSpec, pos token.Position) (string, error) {
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

	return fmt.Sprintf("// From %s.\n%s %s: %s = %s\n\n", pos.String(), valueDecl, valueName, valueType, valueValue), nil
}

func getIdent(e ast.Expr) (*ast.Ident, string, error) {
	switch t := e.(type) {
	case *ast.Ident:
		return t, "", nil
	case *ast.StarExpr:
		i, ok := t.X.(*ast.Ident)
		if !ok {
			return nil, "", xerrors.New("failed to cast star expr to indent")
		}
		return i, "?", nil
	default:
		return nil, "", xerrors.New("unknown expr type")
	}
}

func toTsType(fieldType string) string {
	switch fieldType {
	case "bool":
		return "boolean"
	case "uint64", "uint32", "float64":
		return "number"
	}

	return fieldType
}

func toJSONField(field *ast.Field) string {
	if field.Tag != nil && field.Tag.Value != "" {
		fieldName := strings.Trim(field.Tag.Value, "`")
		for _, pair := range strings.Split(fieldName, " ") {
			if strings.Contains(pair, `json:`) {
				fieldName := strings.TrimPrefix(pair, `json:`)
				fieldName = strings.Trim(fieldName, `"`)
				fieldName = strings.Split(fieldName, ",")[0]

				return fieldName
			}
		}
	}

	return ""
}
