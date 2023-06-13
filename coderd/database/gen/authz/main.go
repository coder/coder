package main

import (
	"go/format"
	"go/token"
	"log"
	"os"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/decorator/resolver/goast"
	"github.com/dave/dst/decorator/resolver/guess"
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

	dbauthz, err := os.ReadFile("./dbauthz/dbauthz.go")
	if err != nil {
		return xerrors.Errorf("read dbauthz: %w", err)
	}

	// Required to preserve imports!
	f, err := decorator.NewDecoratorWithImports(token.NewFileSet(), "dbauthz", goast.New()).Parse(dbauthz)
	if err != nil {
		return xerrors.Errorf("parse dbauthz: %w", err)
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
		if !ok || ident.Name != "querier" {
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
				Type: &dst.FuncType{
					Func:       true,
					TypeParams: fn.Func.TypeParams,
					Params:     fn.Func.Params,
					Results:    fn.Func.Results,
					Decs:       fn.Func.Decs,
				},
				Recv: &dst.FieldList{
					List: []*dst.Field{{
						Names: []*dst.Ident{dst.NewIdent("q")},
						Type:  dst.NewIdent("*querier"),
					}},
				},
				Decs: dst.FuncDeclDecorations{
					NodeDecs: dst.NodeDecs{
						Before: dst.EmptyLine,
						After:  dst.EmptyLine,
					},
				},
				Body: &dst.BlockStmt{
					List: []dst.Stmt{
						&dst.ExprStmt{
							X: &dst.CallExpr{
								Fun: &dst.Ident{
									Name: "panic",
								},
								Args: []dst.Expr{
									&dst.BasicLit{
										Kind:  token.STRING,
										Value: "\"Not implemented\"",
									},
								},
							},
						},
					},
				},
			}
		}
		f.Decls = append(f.Decls, decl)
	}

	file, err := os.OpenFile("./dbauthz/dbauthz.go", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return xerrors.Errorf("open dbauthz: %w", err)
	}
	defer file.Close()

	// Required to preserve imports!
	restorer := decorator.NewRestorerWithImports("dbauthz", guess.New())
	restored, err := restorer.RestoreFile(f)
	if err != nil {
		return xerrors.Errorf("restore dbauthz: %w", err)
	}
	err = format.Node(file, restorer.Fset, restored)
	return err
}

type storeMethod struct {
	Name string
	Func *dst.FuncType
}

func readStoreInterface() ([]storeMethod, error) {
	querier, err := os.ReadFile("./querier.go")
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

		for _, t := range []*dst.FieldList{funcType.Params, funcType.Results} {
			if t == nil {
				continue
			}
			for _, f := range t.List {
				ident, ok := f.Type.(*dst.Ident)
				if !ok {
					continue
				}
				if !ident.IsExported() {
					continue
				}
				ident.Path = "github.com/coder/coder/coderd/database"
			}
		}

		funcs = append(funcs, storeMethod{
			Name: method.Names[0].Name,
			Func: funcType,
		})
	}
	return funcs, nil
}
