package main

import (
	"context"
	"fmt"
	"go/types"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"cdr.dev/slog/sloggers/sloghuman"

	"golang.org/x/tools/go/packages"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

const (
	baseDir = "./codersdk"
)

func main() {
	ctx := context.Background()
	log := slog.Make(sloghuman.Sink(os.Stderr))
	codeBlocks, err := GenerateFromDirectory(ctx, log, baseDir)
	if err != nil {
		log.Fatal(ctx, err.Error())
	}

	// Just cat the output to a file to capture it
	fmt.Println(codeBlocks.String())
}

type TypescriptTypes struct {
	// Each entry is the type name, and it's typescript code block.
	Types map[string]string
	Enums map[string]string
}

// String just combines all the codeblocks. I store them in a map for unit testing purposes
func (t TypescriptTypes) String() string {
	var s strings.Builder
	for _, v := range t.Types {
		s.WriteString(v)
		s.WriteRune('\n')
	}

	for _, v := range t.Enums {
		s.WriteString(v)
		s.WriteRune('\n')
	}
	return s.String()
}

// GenerateFromDirectory will return all the typescript code blocks for a directory
func GenerateFromDirectory(ctx context.Context, log slog.Logger, directory string) (*TypescriptTypes, error) {
	g := Generator{
		log: log,
	}
	err := g.parsePackage(ctx, directory)
	if err != nil {
		return nil, xerrors.Errorf("parse package %q: %w", directory, err)
	}

	codeBlocks, err := g.generateAll()
	if err != nil {
		return nil, xerrors.Errorf("parse package %q: %w", directory, err)
	}

	return codeBlocks, nil
}

type Generator struct {
	// Package we are scanning.
	pkg *packages.Package
	log slog.Logger
}

// parsePackage takes a list of patterns such as a directory, and parses them.
func (g *Generator) parsePackage(ctx context.Context, patterns ...string) error {
	cfg := &packages.Config{
		// Just accept the fact we need these flags for what we want. Feel free to add
		// more, it'll just increase the time it takes to parse.
		Mode: packages.NeedTypes | packages.NeedName | packages.NeedTypesInfo |
			packages.NeedTypesSizes | packages.NeedSyntax,
		Tests:   false,
		Context: ctx,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return xerrors.Errorf("load package: %w", err)
	}

	// Only support 1 package for now. We can expand it if we need later, we
	// just need to hook up multiple packages in the generator.
	if len(pkgs) != 1 {
		return xerrors.Errorf("expected 1 package, found %d", len(pkgs))
	}

	g.pkg = pkgs[0]
	return nil
}

type Generated struct {
}

// generateAll will generate for all types found in the pkg
func (g *Generator) generateAll() (*TypescriptTypes, error) {
	structs := make(map[string]string)
	enums := make(map[string]types.Object)
	constants := make(map[string][]*types.Const)

	for _, n := range g.pkg.Types.Scope().Names() {
		obj := g.pkg.Types.Scope().Lookup(n)
		if obj == nil || obj.Type() == nil {
			// This would be weird, but it is if the package does not have the type def.
			continue
		}

		switch obj.(type) {
		// All named types are type declarations
		case *types.TypeName:
			named, ok := obj.Type().(*types.Named)
			if !ok {
				panic("all typename should be named types")
			}
			switch named.Underlying().(type) {
			case *types.Struct:
				// Structs are obvious
				st := obj.Type().Underlying().(*types.Struct)
				codeBlock, err := g.buildStruct(obj, st)
				if err != nil {
					return nil, xerrors.Errorf("generate %q: %w", obj.Name())
				}
				structs[obj.Name()] = codeBlock
			case *types.Basic:
				// These are enums. Store to expand later.
				enums[obj.Name()] = obj
			}
		case *types.Var:
			// TODO: Are any enums var declarations?
			v := obj.(*types.Var)
			var _ = v
		case *types.Const:
			c := obj.(*types.Const)
			// We only care about named constant types, since they are enums
			if named, ok := c.Type().(*types.Named); ok {
				name := named.Obj().Name()
				constants[name] = append(constants[name], c)
			}
		}
	}

	// Write all enums
	enumCodeBlocks := make(map[string]string)
	for name, v := range enums {
		var values []string
		for _, elem := range constants[name] {
			// TODO: If we have non string constants, we need to handle that
			//		here.
			values = append(values, elem.Val().String())
		}
		var s strings.Builder
		s.WriteString(g.posLine(v))
		s.WriteString(fmt.Sprintf("export type %s = %s\n",
			name, strings.Join(values, " | "),
		))
		s.WriteRune('\n')

		enumCodeBlocks[name] = s.String()
	}

	return &TypescriptTypes{
		Types: structs,
		Enums: enumCodeBlocks,
	}, nil
}

func (g *Generator) posLine(obj types.Object) string {
	file := g.pkg.Fset.File(obj.Pos())
	position := file.Position(obj.Pos())
	position.Filename = filepath.Join("codersdk", filepath.Base(position.Filename))
	return fmt.Sprintf("// From %s\n",
		position.String(),
	)
}

// buildStruct just prints the typescript def for a type.
func (g *Generator) buildStruct(obj types.Object, st *types.Struct) (string, error) {
	var s strings.Builder
	s.WriteString(g.posLine(obj))

	s.WriteString(fmt.Sprintf("export interface %s {\n", obj.Name()))
	// For each field in the struct, we print 1 line of the typescript interface
	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		tag := reflect.StructTag(st.Tag(i))

		// Use the json name if present
		jsonName := tag.Get("json")
		arr := strings.Split(jsonName, ",")
		jsonName = arr[0]
		if jsonName == "" {
			jsonName = field.Name()
		}

		var tsType string
		var comment string
		// If a `typescript:"string"` exists, we take this, and do not try to infer.
		typescriptTag := tag.Get("typescript")
		if typescriptTag == "-" {
			// Ignore this field
			continue
		} else if typescriptTag != "" {
			tsType = typescriptTag
		} else {
			var err error
			tsType, comment, err = g.typescriptType(obj, field.Type())
			if err != nil {
				return "", xerrors.Errorf("typescript type: %w", err)
			}
		}

		if comment != "" {
			s.WriteString(fmt.Sprintf("\t// %s\n", comment))
		}
		s.WriteString(fmt.Sprintf("\treadonly %s: %s\n", jsonName, tsType))
	}
	s.WriteString("}\n")
	return s.String(), nil
}

// typescriptType this function returns a typescript type for a given
// golang type.
// Eg:
//	[]byte returns "string"
func (g *Generator) typescriptType(obj types.Object, ty types.Type) (string, string, error) {
	switch ty.(type) {
	case *types.Basic:
		bs := ty.(*types.Basic)
		// All basic literals (string, bool, int, etc).
		// TODO: Actually ensure the golang names are ok, otherwise,
		//		we want to put another switch to capture these types
		//		and rename to typescript.
		switch {
		case bs.Info() == types.IsNumeric:
			return "number", "", nil
		case bs.Info() == types.IsBoolean:
			return "boolean", "", nil
		case bs.Kind() == types.Byte:
			// TODO: @emyrk What is a byte for typescript? A string? A uint8?
			return "byte", "", nil
		default:
			return bs.Name(), "", nil
		}
	case *types.Struct:
		// TODO: This kinda sucks right now. It just dumps the struct def
		return ty.String(), "Unknown struct, this might not work", nil
	case *types.Map:
		// TODO: Typescript dictionary??? Object?
		return "map_not_implemented", "", nil
	case *types.Slice, *types.Array:
		// Slice/Arrays are pretty much the same.
		type hasElem interface {
			Elem() types.Type
		}

		arr := ty.(hasElem)
		switch {
		// When type checking here, just use the string. You can cast it
		// to a types.Basic and get the kind if you want too :shrug:
		case arr.Elem().String() == "byte":
			// All byte arrays are strings on the typescript.
			// Is this ok?
			return "string", "", nil
		default:
			// By default, just do an array of the underlying type.
			underlying, comment, err := g.typescriptType(obj, arr.Elem())
			if err != nil {
				return "", "", xerrors.Errorf("array: %w", err)
			}
			return underlying + "[]", comment, nil
		}
	case *types.Named:
		n := ty.(*types.Named)
		// First see if the type is defined elsewhere. If it is, we can just
		// put the name as it will be defined in the typescript codeblock
		// we generate.
		name := n.Obj().Name()
		if obj := g.pkg.Types.Scope().Lookup(n.String()); obj != nil && obj.Name() != name {
			// Sweet! Using other typescript types as fields. This could be an
			// enum or another struct
			return name, "", nil
		}

		// If it's a struct, just use the name of the struct type
		if _, ok := n.Underlying().(*types.Struct); ok {
			return name, "Unknown named type, this might not work", nil
		}

		// Defer to the underlying type.
		return g.typescriptType(obj, ty.Underlying())
	case *types.Pointer:
		// Dereference pointers.
		// TODO: Nullable fields? We could say these fields can be null in the
		//		typescript.
		pt := ty.(*types.Pointer)
		return g.typescriptType(obj, pt.Elem())
	}

	// These are all the other types we need to support.
	// time.Time, uuid, etc.
	return "", "", xerrors.Errorf("unknown type: %s", ty.String())
}
