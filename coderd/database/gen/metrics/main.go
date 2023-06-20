package main

import (
	"fmt"
	"go/format"
	"go/token"
	"log"
	"os"
	"strings"

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

	dbmetrics, err := os.ReadFile("./dbmetrics/dbmetrics.go")
	if err != nil {
		return xerrors.Errorf("read dbfake: %w", err)
	}

	// Required to preserve imports!
	f, err := decorator.NewDecoratorWithImports(token.NewFileSet(), "dbmetrics", goast.New()).Parse(dbmetrics)
	if err != nil {
		return xerrors.Errorf("parse dbfake: %w", err)
	}

	for i := 0; i < len(f.Decls); i++ {
		funcDecl, ok := f.Decls[i].(*dst.FuncDecl)
		if !ok || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			continue
		}
		// Check if the receiver is the struct we're interested in
		_, ok = funcDecl.Recv.List[0].Type.(*dst.Ident)
		if !ok {
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
			params := make([]string, 0)
			if fn.Func.Params != nil {
				for _, p := range fn.Func.Params.List {
					for _, name := range p.Names {
						params = append(params, name.Name)
					}
				}
			}
			returns := make([]string, 0)
			if fn.Func.Results != nil {
				for i := range fn.Func.Results.List {
					returns = append(returns, fmt.Sprintf("r%d", i))
				}
			}

			code := fmt.Sprintf(`
package stub

func stub() {
	start := time.Now()
	%s := m.s.%s(%s)
	m.queryLatencies.WithLabelValues("%s").Observe(time.Since(start).Seconds())
	return %s
}
`, strings.Join(returns, ","), fn.Name, strings.Join(params, ","), fn.Name, strings.Join(returns, ","))
			file, err := decorator.Parse(code)
			if err != nil {
				return xerrors.Errorf("parse code: %w", err)
			}
			stmt, ok := file.Decls[0].(*dst.FuncDecl)
			if !ok {
				return xerrors.Errorf("not ok %T", file.Decls[0])
			}

			// Not implemented!
			// When a function isn't implemented, we automatically stub it!
			decl = &dst.FuncDecl{
				Name: dst.NewIdent(fn.Name),
				Type: fn.Func,
				Recv: &dst.FieldList{
					List: []*dst.Field{{
						Names: []*dst.Ident{dst.NewIdent("m")},
						Type:  dst.NewIdent("metricsStore"),
					}},
				},
				Decs: dst.FuncDeclDecorations{
					NodeDecs: dst.NodeDecs{
						Before: dst.EmptyLine,
						After:  dst.EmptyLine,
					},
				},
				Body: stmt.Body,
			}
		}
		f.Decls = append(f.Decls, decl)
	}

	file, err := os.OpenFile("./dbmetrics/dbmetrics.go", os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0o755)
	if err != nil {
		return xerrors.Errorf("open dbfake: %w", err)
	}
	defer file.Close()

	// Required to preserve imports!
	restorer := decorator.NewRestorerWithImports("dbmetrics", guess.New())
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
