package main

import (
	"go/ast"
	"go/token"
	"go/types"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/tools/go/analysis"
)

// Analyzer reports writes to the chat_messages table outside the chatstate
// package.
//
// chats.snapshot_version and chats.history_version stay consistent only
// while every chat_messages write happens inside a chatstate transition.
// The transition bumps snapshot_version under the row lock and publishes a
// state update; the chat_messages trigger then sets history_version to that
// snapshot. A write from anywhere else moves history_version alone, so the
// chat runner and the chat stream see a row whose version has not changed
// and ignore it. One such writer left a running chat with no task until a
// user acted (CODAGT-749). This check turns the next one into a lint error.
var Analyzer = &analysis.Analyzer{
	Name: "chathistorycheck",
	Doc:  "report chat_messages writes outside the chatstate package",
	Run:  run,
	// ResultType must be set so run can return a typed nil instead of
	// nil, nil, which the nilnil linter forbids. No downstream analyzer
	// depends on this result.
	ResultType: reflect.TypeOf((*struct{})(nil)),
}

const (
	databasePackage  = "github.com/coder/coder/v2/coderd/database"
	chatstatePackage = "github.com/coder/coder/v2/coderd/x/chatd/chatstate"
)

// allowedPackages may call the writers: the package that defines the
// queries, its wrappers, the test seeding helpers, and chatstate, which owns
// the transitions.
var allowedPackages = map[string]bool{
	databasePackage:                true,
	databasePackage + "/dbauthz":   true,
	databasePackage + "/dbgen":     true,
	databasePackage + "/dbmetrics": true,
	databasePackage + "/dbmock":    true,
	chatstatePackage:               true,
}

// chatMessageWriters lists every generated database.Store method whose SQL
// changes chat_messages in a way the history trigger counts. The search
// index backfill queries update only search_tsv columns, which the trigger
// ignores, so they are not listed. TestWritersMatchGeneratedQueries fails
// when this list and coderd/database/queries.sql.go disagree.
var chatMessageWriters = map[string]bool{
	"InsertChatMessages":            true,
	"SoftDeleteChatMessageByID":     true,
	"SoftDeleteChatMessagesAfterID": true,
	"SoftDeleteContextFileMessages": true,
}

// chatMessagesWriteSQL matches SQL that writes chat_messages. Reads such as
// "FROM chat_messages" and tables with a longer name such as
// chat_messages_archive do not match.
var chatMessagesWriteSQL = regexp.MustCompile(`(?is)\b(?:insert\s+into|update|delete\s+from)\s+(?:only\s+)?(?:public\.)?chat_messages\b`)

func run(pass *analysis.Pass) (any, error) {
	if allowedPackage(pass.Pkg.Path()) {
		return (*struct{})(nil), nil
	}
	for _, file := range pass.Files {
		if strings.HasSuffix(pass.Fset.File(file.Pos()).Name(), "_test.go") {
			continue
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CallExpr:
				if name, ok := storeWriterCall(pass, node); ok {
					pass.Reportf(node.Pos(), "%s writes chat_messages outside a chatstate transition; move the write into %s", name, chatstatePackage)
				}
			case *ast.BasicLit:
				if node.Kind != token.STRING {
					return true
				}
				text, err := strconv.Unquote(node.Value)
				if err != nil {
					return true
				}
				if chatMessagesWriteSQL.MatchString(text) {
					pass.Reportf(node.Pos(), "SQL writes chat_messages outside a chatstate transition; move the write into %s", chatstatePackage)
				}
			}
			return true
		})
	}
	return (*struct{})(nil), nil
}

// allowedPackage reports whether pkgPath may write chat_messages. External
// test packages carry a _test suffix and are skipped like test files.
func allowedPackage(pkgPath string) bool {
	return allowedPackages[pkgPath] || strings.HasSuffix(pkgPath, "_test")
}

// storeWriterCall reports whether call invokes one of chatMessageWriters on
// a value whose method is declared in the database package, and returns the
// method name.
func storeWriterCall(pass *analysis.Pass, call *ast.CallExpr) (string, bool) {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return "", false
	}
	fn, ok := pass.TypesInfo.Uses[sel.Sel].(*types.Func)
	if !ok || fn.Pkg() == nil || fn.Pkg().Path() != databasePackage {
		return "", false
	}
	if !chatMessageWriters[fn.Name()] {
		return "", false
	}
	return fn.Name(), true
}
