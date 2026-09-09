package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/tools/go/analysis/analysistest"
)

func TestAnalyzer(t *testing.T) {
	t.Parallel()

	analysistest.Run(t, analysistest.TestData(), Analyzer, "example", chatstatePackage)
}

// searchIndexWriters update only search_tsv and search_tsv_config. The
// chat_messages history trigger ignores those columns, so these queries do
// not move history_version and are not chat history writers. The trigger
// tests in coderd/x/chatd/chatstate pin that exclusion.
var searchIndexWriters = map[string]bool{
	"BackfillChatMessagesSearchTsv":     true,
	"ReindexStaleChatMessagesSearchTsv": true,
}

var queryNamePattern = regexp.MustCompile(`^-- name: (\w+) `)

// TestWritersMatchGeneratedQueries fails when a query that writes
// chat_messages is added to coderd/database/queries without being classified
// here, so the analyzer cannot silently miss a new writer.
func TestWritersMatchGeneratedQueries(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filepath.Join("..", "..", "coderd", "database", "queries.sql.go"), nil, parser.SkipObjectResolution)
	require.NoError(t, err)

	generated := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			valueSpec, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for _, value := range valueSpec.Values {
				lit, ok := value.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				query, err := strconv.Unquote(lit.Value)
				require.NoError(t, err)
				if !chatMessagesWriteSQL.MatchString(query) {
					continue
				}
				match := queryNamePattern.FindStringSubmatch(query)
				require.NotNil(t, match, "generated query without a name header: %q", query)
				generated[match[1]] = true
			}
		}
	}

	classified := map[string]bool{}
	for name := range chatMessageWriters {
		classified[name] = true
	}
	for name := range searchIndexWriters {
		require.False(t, chatMessageWriters[name], "%s is classified as both a history writer and a search index writer", name)
		classified[name] = true
	}
	require.Equal(t, classified, generated,
		"classify every chat_messages write query in chatMessageWriters (moves history_version) or searchIndexWriters (search_tsv columns only)")
}
