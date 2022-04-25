package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"sort"
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
	var (
		astFiles []*ast.File
		enums    = make(map[string]string)
	)
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

		astFiles = append(astFiles, astFile)
	}

	// TypeSpec case for structs and type alias
	loopSpecs(astFiles, func(spec ast.Spec) {
		pos := fset.Position(spec.Pos())
		s, ok := spec.(*ast.TypeSpec)
		if !ok {
			return
		}
		out, err := handleTypeSpec(s, pos, enums)
		if err != nil {
			return
		}

		_, _ = fmt.Printf(out)
	})

	// ValueSpec case for loading type alias values into the enum map
	loopSpecs(astFiles, func(spec ast.Spec) {
		s, ok := spec.(*ast.ValueSpec)
		if !ok {
			return
		}
		handleValueSpec(s, enums)
	})

	// sort keys so output is always the same
	var keys []string
	for k := range enums {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// write each type alias declaration with possible values
	for _, k := range keys {
		_, _ = fmt.Printf("%s\n", enums[k])
	}

	return nil
}

func loopSpecs(astFiles []*ast.File, fn func(spec ast.Spec)) {
	for _, astFile := range astFiles {
		// loop each declaration in file
		for _, node := range astFile.Decls {
			genDecl, ok := node.(*ast.GenDecl)
			if !ok {
				continue
			}
			for _, spec := range genDecl.Specs {
				fn(spec)
			}
		}
	}
}

func handleTypeSpec(typeSpec *ast.TypeSpec, pos token.Position, enums map[string]string) (string, error) {
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
			if fieldName == "" {
				continue
			}

			s = fmt.Sprintf("%s  readonly %s%s: %s\n", s, fieldName, optional, fieldType)
			jsonFields++
		}

		// Do not print struct if it has no json fields
		if jsonFields == 0 {
			return "", xerrors.New("no json fields")
		}

		return fmt.Sprintf("%s}\n\n", s), nil
	// Type alias declaration
	case *ast.Ident:
		// save type declaration to map of types
		// later we come back and add union types to this declaration
		enums[typeSpec.Name.Name] = fmt.Sprintf("%sexport type %s = \n", s, typeSpec.Name.Name)
		return "", xerrors.New("enums are not printed at this stage")
	default:
		return "", xerrors.New("not struct or alias")
	}
}

func handleValueSpec(valueSpec *ast.ValueSpec, enums map[string]string) {
	valueValue := ""
	i, ok := valueSpec.Type.(*ast.Ident)
	if !ok {
		return
	}
	valueType := i.Name

	for _, value := range valueSpec.Values {
		bl, ok := value.(*ast.BasicLit)
		if !ok {
			return
		}
		valueValue = bl.Value
		break
	}

	enums[valueType] = fmt.Sprintf("%s  | %s\n", enums[valueType], valueValue)
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
	case "int", "int8", "int16", "int32", "int64", "uint", "uint8", "uint16", "uint32", "uint64", "uintptr", "float32", "float64":
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
