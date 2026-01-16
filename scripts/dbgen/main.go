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
		_, _ = fmt.Fprintf(os.Stderr, "error: %s\n", err)
		os.Exit(1)
	}
}

func run() error {
	localPath, err := localFilePath()
	if err != nil {
		return err
	}
	databasePath := filepath.Join(localPath, "..", "..", "..", "coderd", "database")
	err = orderAndStubDatabaseFunctions(filepath.Join(databasePath, "dbmetrics", "querymetrics.go"), "m", "queryMetricsStore", func(params stubParams) string {
		return fmt.Sprintf(`
start := time.Now()
%s := m.s.%s(%s)
m.queryLatencies.WithLabelValues("%s").Observe(time.Since(start).Seconds())
m.queryCounts.WithLabelValues(httpmw.ExtractHTTPRoute(ctx), httpmw.ExtractHTTPMethod(ctx), "%s").Inc()
return %s
`, params.Returns, params.FuncName, params.Parameters, params.FuncName, params.FuncName, params.Returns)
	})
	if err != nil {
		return xerrors.Errorf("stub dbmetrics: %w", err)
	}

	err = orderAndStubDatabaseFunctions(filepath.Join(databasePath, "dbauthz", "dbauthz.go"), "q", "querier", func(_ stubParams) string {
		return `panic("not implemented")`
	})
	if err != nil {
		return xerrors.Errorf("stub dbauthz: %w", err)
	}

	err = generateUniqueConstraints()
	if err != nil {
		return xerrors.Errorf("generate unique constraints: %w", err)
	}

	err = generateForeignKeyConstraints()
	if err != nil {
		return xerrors.Errorf("generate foreign key constraints: %w", err)
	}

	err = generateCheckConstraints()
	if err != nil {
		return xerrors.Errorf("generate check constraints: %w", err)
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
// structName is the name of the struct that contains the functions.
// stub is a string that will be used to stub the functions.
func orderAndStubDatabaseFunctions(filePath, receiver, structName string, stub func(params stubParams) string) error {
	declByName := map[string]*dst.FuncDecl{}
	packageName := filepath.Base(filepath.Dir(filePath))

	contents, err := os.ReadFile(filePath)
	if err != nil {
		return xerrors.Errorf("read file: %w", err)
	}

	// Required to preserve imports!
	f, err := decorator.NewDecoratorWithImports(token.NewFileSet(), packageName, goast.New()).Parse(contents)
	if err != nil {
		return xerrors.Errorf("parse file: %w", err)
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
		if ident == nil || ident.Name != structName {
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

		decl, ok := declByName[fn.Name]
		if !ok {
			typeName := structName
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
				Body: &dst.BlockStmt{
					List: append(bodyStmts, funcDecl.Body.List...),
				},
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
		Comments: true,
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
	f, err := parseDBFile("querier.go")
	if err != nil {
		return nil, xerrors.Errorf("parse querier.go: %w", err)
	}
	funcs, err := loadInterfaceFuncs(f, "sqlcQuerier")
	if err != nil {
		return nil, xerrors.Errorf("load interface %s funcs: %w", "sqlcQuerier", err)
	}

	customFile, err := parseDBFile("modelqueries.go")
	if err != nil {
		return nil, xerrors.Errorf("parse modelqueriers.go: %w", err)
	}
	// Custom funcs should be appended after the regular functions
	customFuncs, err := loadInterfaceFuncs(customFile, "customQuerier")
	if err != nil {
		return nil, xerrors.Errorf("load interface %s funcs: %w", "customQuerier", err)
	}

	return append(funcs, customFuncs...), nil
}

func parseDBFile(filename string) (*dst.File, error) {
	localPath, err := localFilePath()
	if err != nil {
		return nil, err
	}

	querierPath := filepath.Join(localPath, "..", "..", "..", "coderd", "database", filename)
	querierData, err := os.ReadFile(querierPath)
	if err != nil {
		return nil, xerrors.Errorf("read %s: %w", filename, err)
	}
	f, err := decorator.Parse(querierData)
	return f, err
}

func loadInterfaceFuncs(f *dst.File, interfaceName string) ([]querierFunction, error) {
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
			if typeSpec.Name.Name != interfaceName {
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
	allMethods := interfaceMethods(querier)
	for _, method := range allMethods {
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
				case *dst.StarExpr:
					ident, ok = t.X.(*dst.Ident)
					if !ok {
						continue
					}
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
				ident.Path = "github.com/coder/coder/v2/coderd/database"
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

// nameFromSnakeCase converts snake_case to CamelCase.
func nameFromSnakeCase(s string) string {
	var ret string
	for _, ss := range strings.Split(s, "_") {
		switch ss {
		case "id":
			ret += "ID"
		case "ids":
			ret += "IDs"
		case "jwt":
			ret += "JWT"
		case "idx":
			ret += "Index"
		case "api":
			ret += "API"
		case "uuid":
			ret += "UUID"
		case "gitsshkeys":
			ret += "GitSSHKeys"
		case "fkey":
			// ignore
		default:
			ret += strings.Title(ss)
		}
	}
	return ret
}

// interfaceMethods returns all embedded methods of an interface.
func interfaceMethods(i *dst.InterfaceType) []*dst.Field {
	var allMethods []*dst.Field
	for _, field := range i.Methods.List {
		switch fieldType := field.Type.(type) {
		case *dst.FuncType:
			allMethods = append(allMethods, field)
		case *dst.InterfaceType:
			allMethods = append(allMethods, interfaceMethods(fieldType)...)
		case *dst.Ident:
			// Embedded interfaces are Idents -> TypeSpec -> InterfaceType
			// If the embedded interface is not in the parsed file, then
			// the Obj will be nil.
			if fieldType.Obj != nil {
				objDecl, ok := fieldType.Obj.Decl.(*dst.TypeSpec)
				if ok {
					isInterface, ok := objDecl.Type.(*dst.InterfaceType)
					if ok {
						allMethods = append(allMethods, interfaceMethods(isInterface)...)
					}
				}
			}
		}
	}
	return allMethods
}
