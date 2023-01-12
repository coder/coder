package coderdtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

type swaggerComment struct {
	method string
	router string
}

func TestAllEndpointsDocumented(t *testing.T) {
	t.Parallel()

	// TODO parse enterprise
	swaggerComments, err := parseSwaggerComments("..")
	require.NoError(t, err, "can't parse swagger comments")

	_, _, api := NewWithAPI(t, nil)
	chi.Walk(api.APIHandler, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		method = strings.ToLower(method)
		if route != "/" && strings.HasSuffix(route, "/") {
			route = route[:len(route)-1]
		}

		c := findSwaggerCommentByMethodAndRoute(swaggerComments, method, route)
		assert.NotNil(t, c, "Missing @Router annotation for: [%s] %s", method, route)
		return nil
	})
}

func parseSwaggerComments(dir string) ([]swaggerComment, error) {
	fileSet := token.NewFileSet()
	commentNodes, err := parser.ParseDir(fileSet, dir, nil, parser.ParseComments)
	if err != nil {
		return nil, xerrors.Errorf(`parser.ParseDir failed "%s": %w`, dir, err)
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
			args := strings.SplitN(text, " ", 4)
			c.router = args[2]
			c.method = args[3][1 : len(args[3])-1]
		}
	}
	return c
}

func findSwaggerCommentByMethodAndRoute(comments []swaggerComment, method, route string) *swaggerComment {
	for _, c := range comments {
		if c.method == method && c.router == route {
			return &c
		}
	}
	return nil
}
