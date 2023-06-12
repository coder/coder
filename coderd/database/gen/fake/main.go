package main

import (
	"log"
	"os"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"golang.org/x/xerrors"
)

func main() {
	err := run()
	if err != nil {
		log.Fatal(err)
	}
}

func run() error {
	funcs, err := readStoreInterface()
	if err != nil {
		return err
	}
	funcByName := map[string]struct{}{}
	for _, f := range funcs {
		funcByName[f.Name] = struct{}{}
	}
	declByName := map[string]*dst.FuncDecl{}

	dbfake, err := os.ReadFile("../../dbfake/dbfake.go")
	if err != nil {
		return xerrors.Errorf("read dbfake: %w", err)
	}
	f, err := decorator.Parse(dbfake)
	if err != nil {
		return xerrors.Errorf("parse dbfake: %w", err)
	}

	for i := 0; i < len(f.Decls); i++ {
		funcDecl, ok := f.Decls[i].(*dst.FuncDecl)
		if !ok || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			continue
		}
		// Check if the receiver is the struct we're interested in
		starExpr, ok := funcDecl.Recv.List[0].Type.(*dst.StarExpr)
		if !ok {
			continue
		}
		ident, ok := starExpr.X.(*dst.Ident)
		if !ok || ident.Name != "fakeQuerier" {
			continue
		}
		if _, ok := funcByName[funcDecl.Name.Name]; !ok {
			continue
		}
		declByName[funcDecl.Name.Name] = funcDecl
		f.Decls = append(f.Decls[:i], f.Decls[i+1:]...)
		i--
	}

	for _, fn := range funcs {
		decl, ok := declByName[fn.Name]
		if !ok {
			// Not implemented!
			decl = &dst.FuncDecl{
				Name: dst.NewIdent(fn.Name),
				Type: fn.Func,
				Recv: &dst.FieldList{
					List: []*dst.Field{{
						Names: []*dst.Ident{dst.NewIdent("q")},
						Type:  dst.NewIdent("*fakeQuerier"),
					}},
				},
				Decs: dst.FuncDeclDecorations{
					NodeDecs: dst.NodeDecs{
						Before: dst.EmptyLine,
						After:  dst.EmptyLine,
					},
				},
				Body: &dst.BlockStmt{
					Decs: dst.BlockStmtDecorations{
						Lbrace: dst.Decorations{
							"\n",
							"// Implement me!",
						},
					},
				},
			}
		}
		f.Decls = append(f.Decls, decl)
	}

	file, err := os.OpenFile("../../dbfake/dbfake.go", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0755)
	if err != nil {
		return xerrors.Errorf("open dbfake: %w", err)
	}
	defer file.Close()

	err = decorator.Fprint(file, f)
	if err != nil {
		return xerrors.Errorf("write dbfake: %w", err)
	}

	return nil
}

type storeMethod struct {
	Name string
	Func *dst.FuncType
}

func readStoreInterface() ([]storeMethod, error) {
	querier, err := os.ReadFile("../../querier.go")
	if err != nil {
		return nil, xerrors.Errorf("read querier: %w", err)
	}
	f, err := decorator.Parse(querier)
	if err != nil {
		return nil, err
	}

	var sqlcQuerier *dst.InterfaceType
	for _, decl := range f.Decls {
		genDecl, ok := decl.(*dst.GenDecl)
		if !ok {
			continue
		}

		for _, spec := range genDecl.Specs {
			typeSpec, ok := spec.(*dst.TypeSpec)
			if !ok {
				continue
			}
			if typeSpec.Name.Name != "sqlcQuerier" {
				continue
			}
			sqlcQuerier, ok = typeSpec.Type.(*dst.InterfaceType)
			if !ok {
				return nil, xerrors.Errorf("unexpected sqlcQuerier type: %T", typeSpec.Type)
			}
			break
		}
	}
	if sqlcQuerier == nil {
		return nil, xerrors.Errorf("sqlcQuerier not found")
	}
	funcs := []storeMethod{}
	for _, method := range sqlcQuerier.Methods.List {
		funcType, ok := method.Type.(*dst.FuncType)
		if !ok {
			continue
		}
		funcs = append(funcs, storeMethod{
			Name: method.Names[0].Name,
			Func: funcType,
		})
	}
	return funcs, nil
}
