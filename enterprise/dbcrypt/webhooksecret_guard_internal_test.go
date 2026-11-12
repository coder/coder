package dbcrypt

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/database"
)

// dbCrypt encrypts only in methods it declares itself. A database.Store method
// that dbCrypt does not declare is promoted from the embedded store and moves
// the webhook secret in plaintext. This test names every Store method that
// takes or returns a type with a WebhookSecret field, or whose query names the
// column, so a query added later cannot skip its override.
func TestWebhookSecretMethodsAreOverridden(t *testing.T) {
	t.Parallel()

	carriers := storeMethodsWithField(reflect.TypeFor[database.Store](), "WebhookSecret")
	t.Logf("database.Store methods that carry a webhook secret: %v", carriers)
	require.Subset(t, carriers, []string{"InsertChatAutomation", "GetChatAutomationByID"},
		"the walk must find the queries of A1 that carry the secret")
	// sqlc.embed puts a row into a named field.
	type embedding struct {
		Automation struct{ WebhookSecret string }
	}
	require.True(t, typeHasField(reflect.TypeFor[[]embedding](), "WebhookSecret", map[reflect.Type]bool{}))
	named := queriesNamingColumn(t, "webhook_secret")
	require.Subset(t, named, []string{"InsertChatAutomation", "UpdateEncryptedChatAutomationWebhookSecret"},
		"the scan must find the queries that name the column")

	declared := dbCryptMethods(t)
	needed := slices.Concat(carriers, named)
	slices.Sort(needed)
	missing := slices.DeleteFunc(slices.Compact(needed), func(name string) bool {
		return slices.Contains(declared, name)
	})
	require.Empty(t, missing,
		"these database.Store methods carry or name a webhook secret, so dbCrypt must override them to encrypt and decrypt the secret")

	// A list covers every automation of an organization. It must not load a
	// secret, so it has no override and no field to decrypt.
	for _, name := range []string{"GetChatAutomationsByOrganizationID", "GetAuthorizedChatAutomationsByOrganizationID"} {
		require.NotContains(t, carriers, name, "the list must not load a webhook secret")
		require.NotContains(t, named, name, "the list must not load a webhook secret")
	}
}

func storeMethodsWithField(store reflect.Type, field string) []string {
	var names []string
	for i := range store.NumMethod() {
		method := store.Method(i)
		sig := method.Type
		hasField := false
		for j := range sig.NumIn() {
			hasField = hasField || typeHasField(sig.In(j), field, map[reflect.Type]bool{})
		}
		for j := range sig.NumOut() {
			hasField = hasField || typeHasField(sig.Out(j), field, map[reflect.Type]bool{})
		}
		if hasField {
			names = append(names, method.Name)
		}
	}
	slices.Sort(names)
	return names
}

// typeHasField reports whether t, or a struct reachable through its fields,
// has a field with the given name. It finds a secret that sqlc.embed puts
// inside a named field.
func typeHasField(t reflect.Type, field string, seen map[reflect.Type]bool) bool {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice || t.Kind() == reflect.Array {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || seen[t] {
		return false
	}
	seen[t] = true
	if _, ok := t.FieldByName(field); ok {
		return true
	}
	for i := range t.NumField() {
		if typeHasField(t.Field(i).Type, field, seen) {
			return true
		}
	}
	return false
}

// queriesNamingColumn returns the names of the sqlc queries whose SQL names the
// column. It finds a query that returns the bare column or takes the secret in
// a parameter that a struct walk cannot tie to the column.
func queriesNamingColumn(t *testing.T, column string) []string {
	t.Helper()
	files, err := filepath.Glob("../../coderd/database/queries/*.sql")
	require.NoError(t, err)
	require.NotEmpty(t, files)

	header := regexp.MustCompile(`(?m)^-- name: (\w+) :`)
	named := regexp.MustCompile(`\b` + column + `\b`)
	var names []string
	for _, file := range files {
		data, err := os.ReadFile(file)
		require.NoError(t, err)
		text := string(data)
		headers := header.FindAllStringSubmatchIndex(text, -1)
		for i, h := range headers {
			end := len(text)
			if i+1 < len(headers) {
				end = headers[i+1][0]
			}
			var code []string
			for _, line := range strings.Split(text[h[1]:end], "\n") {
				if !strings.HasPrefix(strings.TrimSpace(line), "--") {
					code = append(code, line)
				}
			}
			if named.MatchString(strings.Join(code, "\n")) {
				names = append(names, text[h[2]:h[3]])
			}
		}
	}
	slices.Sort(names)
	return names
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
