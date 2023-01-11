package coderdtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

type swaggerComment struct {
	method string
	router string
}

func TestAllEndpointsDocumented(t *testing.T) {
	t.Parallel()

	swaggerCommentss, err := parseSwaggerComments("..") // TODO enterprise
	require.NoError(t, err, "can't parse swagger comments")

	for _, c := range swaggerCommentss {
		t.Log(c.method, c.router)
	}

	_, _, api := NewWithAPI(t, nil)
	err = chi.Walk(api.APIHandler, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		return nil
	})
	require.Error(t, err)
}

func parseSwaggerComments(dir string) ([]swaggerComment, error) {
	fileSet := token.NewFileSet()
	commentNodes, err := parser.ParseDir(fileSet, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, xerrors.Errorf(`can't parse directory "%s": %w`, dir)
	}

	var swaggerComments []swaggerComment
	for _, commentNode := range commentNodes {
		ast.Inspect(commentNode, func(n ast.Node) bool {
			commentGroup, ok := n.(*ast.CommentGroup)
			if !ok {
				return true
			}

			var isSwaggerComment bool
			for _, line := range commentGroup.List {
				text := strings.TrimSpace(line.Text)
				if strings.HasPrefix(text, "//") && strings.Contains(text, "@Router") {
					isSwaggerComment = true
					break
				}
			}

			if isSwaggerComment {
				swaggerComments = append(swaggerComments, parseSwaggerComment(commentGroup))
			}
			return true
		})
	}
	return swaggerComments, nil
}

func parseSwaggerComment(commentGroup *ast.CommentGroup) swaggerComment {
	var c swaggerComment
	for _, line := range commentGroup.List {
		text := strings.TrimSpace(line.Text)
		if strings.Contains(text, "@Router ") {
			c.router = strings.SplitN(text, " ", 3)[2]
		}
	}
	return c
}
