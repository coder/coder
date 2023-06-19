package main

import (
	"fmt"
	"go/format"
	"go/token"
	"log"
	"os"
	"path"

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

	dbfake, err := os.ReadFile("./dbfake/dbfake.go")
	if err != nil {
		return xerrors.Errorf("read dbfake: %w", err)
	}

	// Required to preserve imports!
	f, err := decorator.NewDecoratorWithImports(token.NewFileSet(), "dbfake", goast.New()).Parse(dbfake)
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
		var bodyStmts []dst.Stmt
		if len(fn.Func.Params.List) == 2 && fn.Func.Params.List[1].Names[0].Name == "arg" {
			/*
				err := validateDatabaseType(arg)
				if err != nil {
					return database.User{}, err
				}
			*/
			bodyStmts = append(bodyStmts, &dst.AssignStmt{
				Lhs: []dst.Expr{dst.NewIdent("err")},
				Tok: token.DEFINE,
				Rhs: []dst.Expr{
					&dst.CallExpr{
						Fun: &dst.Ident{
							Name: "validateDatabaseType",
						},
						Args: []dst.Expr{dst.NewIdent("arg")},
					},
				},
			})
			returnStmt := &dst.ReturnStmt{
				Results: []dst.Expr{}, // Filled below.
			}
			bodyStmts = append(bodyStmts, &dst.IfStmt{
				Cond: &dst.BinaryExpr{
					X:  dst.NewIdent("err"),
					Op: token.NEQ,
					Y:  dst.NewIdent("nil"),
				},
				Body: &dst.BlockStmt{
					List: []dst.Stmt{
						returnStmt,
					},
				},
				Decs: dst.IfStmtDecorations{
					NodeDecs: dst.NodeDecs{
						After: dst.EmptyLine,
					},
				},
			})
			for _, r := range fn.Func.Results.List {
				switch typ := r.Type.(type) {
				case *dst.StarExpr, *dst.ArrayType:
					returnStmt.Results = append(returnStmt.Results, dst.NewIdent("nil"))
				case *dst.Ident:
					if typ.Path != "" {
						returnStmt.Results = append(returnStmt.Results, dst.NewIdent(fmt.Sprintf("%s.%s{}", path.Base(typ.Path), typ.Name)))
					} else {
						switch typ.Name {
						case "uint8", "uint16", "uint32", "uint64", "uint", "uintptr",
							"int8", "int16", "int32", "int64", "int",
							"byte", "rune",
							"float32", "float64",
							"complex64", "complex128":
							returnStmt.Results = append(returnStmt.Results, dst.NewIdent("0"))
						case "string":
							returnStmt.Results = append(returnStmt.Results, dst.NewIdent("\"\""))
						case "bool":
							returnStmt.Results = append(returnStmt.Results, dst.NewIdent("false"))
						case "error":
							returnStmt.Results = append(returnStmt.Results, dst.NewIdent("err"))
						default:
							panic(fmt.Sprintf("unknown ident: %#v", r.Type))
						}
					}
				default:
					panic(fmt.Sprintf("unknown return type: %T", r.Type))
				}
			}
		}
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
					List: append(bodyStmts, &dst.ExprStmt{
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
					}),
				},
			}
		}
		f.Decls = append(f.Decls, decl)
	}

	file, err := os.OpenFile("./dbfake/dbfake.go", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return xerrors.Errorf("open dbfake: %w", err)
	}
	defer file.Close()

	// Required to preserve imports!
	restorer := decorator.NewRestorerWithImports("dbfake", guess.New())
	restored, err := restorer.RestoreFile(f)
	if err != nil {
		return xerrors.Errorf("restore dbfake: %w", err)
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
			var (
				ident *dst.Ident
				ok    bool
			)
			for _, f := range t.List {
				switch typ := f.Type.(type) {
				case *dst.StarExpr:
					ident, ok = typ.X.(*dst.Ident)
					if !ok {
						continue
					}
				case *dst.ArrayType:
					ident, ok = typ.Elt.(*dst.Ident)
					if !ok {
						continue
					}
				case *dst.Ident:
					ident = typ
				default:
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
