package main
import (
	"errors"
	"context"
	"fmt"
	"go/types"
	"io"
	"os"
	"reflect"
	"strings"
	"golang.org/x/tools/go/packages"
	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
)
func main() {
	ctx := context.Background()
	log := slog.Make(sloghuman.Sink(os.Stderr))
	code, err := GenerateFromDirectory(ctx, os.Args[1], os.Args[2:]...)
	if err != nil {
		log.Fatal(ctx, "generate", slog.Error(err))
	}
	_, _ = fmt.Print(code)
}
// GenerateFromDirectory will return all the typescript code blocks for a directory
func GenerateFromDirectory(ctx context.Context, directory string, typeNames ...string) (string, error) {
	g := Generator{}
	err := g.parsePackage(ctx, directory)
	if err != nil {
		return "", fmt.Errorf("parse package %q: %w", directory, err)
	}
	str, err := g.generate(typeNames...)
	if err != nil {
		return "", fmt.Errorf("parse package %q: %w", directory, err)
	}
	return str, nil
}
type Generator struct {
	// Package we are scanning.
	pkg *packages.Package
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
		return fmt.Errorf("load package: %w", err)
	}
	// Only support 1 package for now. We can expand it if we need later, we
	// just need to hook up multiple packages in the generator.
	if len(pkgs) != 1 {
		return fmt.Errorf("expected 1 package, found %d", len(pkgs))
	}
	g.pkg = pkgs[0]
	return nil
}
func (g *Generator) generate(typeNames ...string) (string, error) {
	sb := strings.Builder{}
	_, _ = fmt.Fprint(&sb, "Copy the following code into the audit.AuditableResources table\n\n")
	for _, typName := range typeNames {
		obj := g.pkg.Types.Scope().Lookup(typName)
		if obj == nil || obj.Type() == nil {
			return "", fmt.Errorf("type doesn't exist %q", typName)
		}
		switch obj := obj.(type) {
		case *types.TypeName:
			named, ok := obj.Type().(*types.Named)
			if !ok {
				panic("all typenames should be named types")
			}
			switch typ := named.Underlying().(type) {
			case *types.Struct:
				g.writeStruct(&sb, typ, typName)
			default:
				return "", fmt.Errorf("invalid type %T", obj)
			}
		default:
			return "", fmt.Errorf("invalid type %T", obj)
		}
	}
	return sb.String(), nil
}
func (*Generator) writeStruct(w io.Writer, st *types.Struct, name string) {
	_, _ = fmt.Fprintf(w, "\t&database.%s{}: {\n", name)
	for i := 0; i < st.NumFields(); i++ {
		_, _ = fmt.Fprintf(w, "\t\t\"%s\": ActionIgnore, // TODO: why\n", reflect.StructTag(st.Tag(i)).Get("json"))
	}
	_, _ = fmt.Fprint(w, "\t},\n")
}
