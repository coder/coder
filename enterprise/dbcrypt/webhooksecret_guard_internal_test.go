package dbcrypt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

// dbCrypt encrypts only in methods it declares itself. A database.Store method
// that dbCrypt does not declare is promoted from the embedded store and moves
// the webhook secret in plaintext. This test names every Store method that
// takes or returns a type with a WebhookSecret field, so a query added later
// cannot skip its override.
func TestWebhookSecretMethodsAreOverridden(t *testing.T) {
	t.Parallel()

	carriers := storeMethodsWithField(reflect.TypeFor[database.Store](), "WebhookSecret")
	t.Logf("database.Store methods that carry a webhook secret: %v", carriers)
	require.Subset(t, carriers, []string{"InsertChatAutomation", "GetChatAutomationByID"},
		"the walk must find the queries of A1 that carry the secret")

	declared := dbCryptMethods(t)
	missing := slices.DeleteFunc(slices.Clone(carriers), func(name string) bool {
		return slices.Contains(declared, name)
	})
	require.Empty(t, missing,
		"these database.Store methods take or return a WebhookSecret, so dbCrypt must override them to encrypt and decrypt the secret")

	// A list covers every automation of an organization. It must not load a
	// secret, so it has no override and no field to decrypt.
	for _, name := range []string{"GetChatAutomationsByOrganizationID", "GetAuthorizedChatAutomationsByOrganizationID"} {
		require.NotContains(t, carriers, name, "the list must not load a webhook secret")
	}
}

func storeMethodsWithField(store reflect.Type, field string) []string {
	var names []string
	for i := range store.NumMethod() {
		method := store.Method(i)
		sig := method.Type
		hasField := false
		for j := range sig.NumIn() {
			hasField = hasField || typeHasField(sig.In(j), field)
		}
		for j := range sig.NumOut() {
			hasField = hasField || typeHasField(sig.Out(j), field)
		}
		if hasField {
			names = append(names, method.Name)
		}
	}
	slices.Sort(names)
	return names
}

func typeHasField(t reflect.Type, field string) bool {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct {
		return false
	}
	_, ok := t.FieldByName(field)
	return ok
}

// dbCryptMethods returns the names of the methods that the non-test files of
// this package declare on dbCrypt.
func dbCryptMethods(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(".")
	require.NoError(t, err)

	var names []string
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, entry.Name(), nil, parser.SkipObjectResolution)
		require.NoError(t, err)
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 {
				continue
			}
			recv := fn.Recv.List[0].Type
			if star, ok := recv.(*ast.StarExpr); ok {
				recv = star.X
			}
			if ident, ok := recv.(*ast.Ident); ok && ident.Name == "dbCrypt" {
				names = append(names, fn.Name.Name)
			}
		}
	}
	require.NotEmpty(t, names)
	return names
}
