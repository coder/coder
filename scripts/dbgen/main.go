package main

import (
	"bytes"
	"fmt"
	"go/format"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"

	"github.com/dave/dst"
	"github.com/dave/dst/decorator"
	"github.com/dave/dst/decorator/resolver/goast"
	"github.com/dave/dst/decorator/resolver/guess"
	"golang.org/x/tools/imports"
	"golang.org/x/xerrors"
)

var (
	funcs      []querierFunction
	funcByName map[string]struct{}
)

func init() {
	var err error
	funcs, err = readQuerierFunctions()
	if err != nil {
		panic(err)
	}
	funcByName = map[string]struct{}{}
	for _, f := range funcs {
		funcByName[f.Name] = struct{}{}
	}
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	localPath, err := localFilePath()
	if err != nil {
		return err
	}
	databasePath := filepath.Join(localPath, "..", "..", "..", "coderd", "database")

	err = orderAndStubDatabaseFunctions(filepath.Join(databasePath, "dbfake", "dbfake.go"), "q", "fakeQuerier", func(params stubParams) string {
		return `panic("not implemented")`
	})
	if err != nil {
		return xerrors.Errorf("stub dbfake: %w", err)
	}

	err = orderAndStubDatabaseFunctions(filepath.Join(databasePath, "dbmetrics", "dbmetrics.go"), "m", "metricsStore", func(params stubParams) string {
		return fmt.Sprintf(`
start := time.Now()
%s := m.s.%s(%s)
m.queryLatencies.WithLabelValues("%s").Observe(time.Since(start).Seconds())
return %s
`, params.Returns, params.FuncName, params.Parameters, params.FuncName, params.Returns)
	})
	if err != nil {
		return xerrors.Errorf("stub dbmetrics: %w", err)
	}

	err = orderAndStubDatabaseFunctions(filepath.Join(databasePath, "dbauthz", "dbauthz.go"), "q", "querier", func(params stubParams) string {
		return `panic("not implemented")`
	})
	if err != nil {
		return xerrors.Errorf("stub dbauthz: %w", err)
	}

	return nil
}

type stubParams struct {
	FuncName   string
	Parameters string
	Returns    string
}

// orderAndStubDatabaseFunctions orders the functions in the file and stubs them.
// This is useful for when we want to add a new function to the database and
// we want to make sure that it's ordered correctly.
//
// querierFuncs is a list of functions that are in the database.
// file is the path to the file that contains all the functions.
// struc is the name of the struct that contains the functions.
// stub is a string that will be used to stub the functions.
func orderAndStubDatabaseFunctions(filePath, receiver, struc string, stub func(params stubParams) string) error {
	declByName := map[string]*dst.FuncDecl{}
	packageName := filepath.Base(filepath.Dir(filePath))

	contents, err := os.ReadFile(filePath)
	if err != nil {
		return xerrors.Errorf("read dbfake: %w", err)
	}

	// Required to preserve imports!
	f, err := decorator.NewDecoratorWithImports(token.NewFileSet(), packageName, goast.New()).Parse(contents)
	if err != nil {
		return xerrors.Errorf("parse dbfake: %w", err)
	}

	pointer := false
	for i := 0; i < len(f.Decls); i++ {
		funcDecl, ok := f.Decls[i].(*dst.FuncDecl)
		if !ok || funcDecl.Recv == nil || len(funcDecl.Recv.List) == 0 {
			continue
		}

		var ident *dst.Ident
		switch t := funcDecl.Recv.List[0].Type.(type) {
		case *dst.Ident:
			ident = t
		case *dst.StarExpr:
			ident, ok = t.X.(*dst.Ident)
			if !ok {
				continue
			}
			pointer = true
		}
		if ident == nil || ident.Name != struc {
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
			typeName := struc
			if pointer {
				typeName = "*" + typeName
			}
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

			funcDecl, err := compileFuncDecl(stub(stubParams{
				FuncName:   fn.Name,
				Parameters: strings.Join(params, ","),
				Returns:    strings.Join(returns, ","),
			}))
			if err != nil {
				return xerrors.Errorf("compile func decl: %w", err)
			}

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
						Names: []*dst.Ident{dst.NewIdent(receiver)},
						Type:  dst.NewIdent(typeName),
					}},
				},
				Decs: dst.FuncDeclDecorations{
					NodeDecs: dst.NodeDecs{
						Before: dst.EmptyLine,
						After:  dst.EmptyLine,
					},
				},
				Body: funcDecl.Body,
			}
		}
		if ok {
			for i, pm := range fn.Func.Params.List {
				if len(decl.Type.Params.List) < i+1 {
					decl.Type.Params.List = append(decl.Type.Params.List, pm)
				}
				if !reflect.DeepEqual(decl.Type.Params.List[i].Type, pm.Type) {
					decl.Type.Params.List[i].Type = pm.Type
				}
			}

			for i, res := range fn.Func.Results.List {
				if len(decl.Type.Results.List) < i+1 {
					decl.Type.Results.List = append(decl.Type.Results.List, res)
				}
				if !reflect.DeepEqual(decl.Type.Results.List[i].Type, res.Type) {
					decl.Type.Results.List[i].Type = res.Type
				}
			}
		}
		f.Decls = append(f.Decls, decl)
	}

	// Required to preserve imports!
	restorer := decorator.NewRestorerWithImports(packageName, guess.New())
	restored, err := restorer.RestoreFile(f)
	if err != nil {
		return xerrors.Errorf("restore package: %w", err)
	}
	var buf bytes.Buffer
	err = format.Node(&buf, restorer.Fset, restored)
	if err != nil {
		return xerrors.Errorf("format package: %w", err)
	}
	data, err := imports.Process(filePath, buf.Bytes(), &imports.Options{
		Comments:   true,
		FormatOnly: true,
	})
	if err != nil {
		return xerrors.Errorf("process imports: %w", err)
	}
	return os.WriteFile(filePath, data, 0o600)
}

// compileFuncDecl extracts the function declaration from the given code.
func compileFuncDecl(code string) (*dst.FuncDecl, error) {
	f, err := decorator.Parse(fmt.Sprintf(`package stub

func stub() {
	%s
}`, strings.TrimSpace(code)))
	if err != nil {
		return nil, err
	}
	if len(f.Decls) != 1 {
		return nil, xerrors.Errorf("expected 1 decl, got %d", len(f.Decls))
	}
	decl, ok := f.Decls[0].(*dst.FuncDecl)
	if !ok {
		return nil, xerrors.Errorf("expected func decl, got %T", f.Decls[0])
	}
	return decl, nil
}

type querierFunction struct {
	// Name is the name of the function. Like "GetUserByID"
	Name string
	// Func is the AST representation of a function.
	Func *dst.FuncType
}

// readQuerierFunctions reads the functions from coderd/database/querier.go
func readQuerierFunctions() ([]querierFunction, error) {
	localPath, err := localFilePath()
	if err != nil {
		return nil, err
	}
	querierPath := filepath.Join(localPath, "..", "..", "..", "coderd", "database", "querier.go")

	querierData, err := os.ReadFile(querierPath)
	if err != nil {
		return nil, xerrors.Errorf("read querier: %w", err)
	}
	f, err := decorator.Parse(querierData)
	if err != nil {
		return nil, err
	}

	var querier *dst.InterfaceType
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
			// This is the name of the interface. If that ever changes,
			// this will need to be updated.
			if typeSpec.Name.Name != "sqlcQuerier" {
				continue
			}
			querier, ok = typeSpec.Type.(*dst.InterfaceType)
			if !ok {
				return nil, xerrors.Errorf("unexpected sqlcQuerier type: %T", typeSpec.Type)
			}
			break
		}
	}
	if querier == nil {
		return nil, xerrors.Errorf("querier not found")
	}
	funcs := []querierFunction{}
	for _, method := range querier.Methods.List {
		funcType, ok := method.Type.(*dst.FuncType)
		if !ok {
			continue
		}

		for _, t := range []*dst.FieldList{funcType.Params, funcType.Results, funcType.TypeParams} {
			if t == nil {
				continue
			}
			for _, f := range t.List {
				var ident *dst.Ident
				switch t := f.Type.(type) {
				case *dst.Ident:
					ident = t
				case *dst.SelectorExpr:
					ident, ok = t.X.(*dst.Ident)
					if !ok {
						continue
					}
				case *dst.ArrayType:
					ident, ok = t.Elt.(*dst.Ident)
					if !ok {
						continue
					}
				}
				if ident == nil {
					continue
				}
				// If the type is exported then we should be able to find it
				// in the database package!
				if !ident.IsExported() {
					continue
				}
				ident.Path = "github.com/coder/coder/coderd/database"
			}
		}

		funcs = append(funcs, querierFunction{
			Name: method.Names[0].Name,
			Func: funcType,
		})
	}
	return funcs, nil
}

// localFilePath returns the location of `main.go` in the dbgen package.
func localFilePath() (string, error) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		return "", xerrors.Errorf("failed to get caller")
	}
	return filename, nil
}
