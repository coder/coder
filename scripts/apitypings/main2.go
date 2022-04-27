package main

import (
	"context"
	"fmt"
	"go/types"
	"reflect"
	"strings"

	"golang.org/x/tools/go/packages"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
)

type Generator struct {
	pkg *packages.Package // Package we are scanning.
	log slog.Logger
}

// parsePackage takes a list of patterns such as a directory, and parses them.
// All parsed packages will accumulate "foundTypes".
func (g *Generator) parsePackage(ctx context.Context, patterns ...string) error {
	cfg := &packages.Config{
		Mode: packages.NeedTypes | packages.NeedName | packages.NeedTypesInfo |
			packages.NeedTypesSizes | packages.NeedSyntax,
		Tests:   false,
		Context: ctx,
	}

	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		return xerrors.Errorf("load package: %w", err)
	}

	if len(pkgs) != 1 {
		return xerrors.Errorf("expected 1 package, found %d", len(pkgs))
	}

	g.pkg = pkgs[0]
	return nil
}

// generateAll will generate for all types found in the pkg
func (g *Generator) generateAll() error {
	for _, n := range g.pkg.Types.Scope().Names() {
		err := g.generate(n)
		if err != nil {
			return xerrors.Errorf("generate %q: %w", n, err)
		}
	}
	return nil
}

// generate generates the typescript for a singular Go type.
func (g *Generator) generate(typeName string) error {
	obj := g.pkg.Types.Scope().Lookup(typeName)
	if obj == nil || obj.Type() == nil {
		return xerrors.Errorf("pkg is missing type %q", typeName)
	}

	st, ok := obj.Type().Underlying().(*types.Struct)
	if !ok {
		return nil
		//return xerrors.Errorf("only generate for structs, found %q", obj.Type().String())
	}

	return g.buildStruct(obj, st)
}

// buildStruct just prints the typescript def for a type.
// TODO: Write to a buffer instead
func (g *Generator) buildStruct(obj types.Object, st *types.Struct) error {
	var s strings.Builder
	s.WriteString("export interface " + obj.Name() + "{\n")
	for i := 0; i < st.NumFields(); i++ {
		field := st.Field(i)
		tag := reflect.StructTag(st.Tag(i))
		jsonName := tag.Get("json")
		arr := strings.Split(jsonName, ",")
		jsonName = arr[0]
		if jsonName == "" {
			jsonName = field.Name()
		}

		ts, err := g.typescriptType(field.Type())
		if err != nil {
			return xerrors.Errorf("typescript type: %w", err)
		}
		s.WriteString(fmt.Sprintf("\treadonly %s: %s\n", jsonName, ts))
	}
	s.WriteString("}")
	fmt.Println(s.String())
	return nil
}

// typescriptType this function returns a typescript type for a given
// golang type.
// Eg:
//	[]byte returns "string"
func (g *Generator) typescriptType(ty types.Type) (string, error) {
	switch ty.(type) {
	case *types.Basic:
		bs := ty.(*types.Basic)
		// All basic literals (string, bool, int, etc).
		// TODO: Actually ensure the golang names are ok, otherwise,
		//		we want to put another switch to capture these types
		//		and rename to typescript.
		return bs.Name(), nil
	case *types.Struct:
		// TODO: This kinda sucks right now. It just dumps the struct def
		return ty.String(), nil
	case *types.Map:
		// TODO: Typescript dictionary??? Object?
		return "map", nil
	case *types.Slice, *types.Array:
		type hasElem interface {
			Elem() types.Type
		}

		arr := ty.(hasElem)
		// All byte arrays should be strings in typescript?
		if arr.Elem().String() == "byte" {
			return "string", nil
		}

		// Array of underlying type.
		underlying, err := g.typescriptType(arr.Elem())
		if err != nil {
			return "", xerrors.Errorf("array: %w", err)
		}
		return underlying + "[]", nil
	case *types.Named:
		// Named is a named type like
		//	 type EnumExample string
		// Use the underlying type
		n := ty.(*types.Named)
		name := n.Obj().Name()
		// If we have the type, just put the name because it will be defined
		// elsewhere in the typescript gen.
		if obj := g.pkg.Types.Scope().Lookup(n.String()); obj != nil {
			return name, nil
		}

		// If it's a struct, just use the name for now.
		if _, ok := ty.Underlying().(*types.Struct); ok {
			return name, nil
		}

		// Defer to the underlying type.
		return g.typescriptType(ty.Underlying())
	case *types.Pointer:
		// Dereference pointers.
		// TODO: Nullable fields?
		pt := ty.(*types.Pointer)
		return g.typescriptType(pt.Elem())
	}

	// These are all the other types we need to support.
	// time.Time, uuid, etc.
	return "", xerrors.Errorf("unknown type: %s", ty.String())
}
