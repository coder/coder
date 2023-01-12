package coderdtest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

type swaggerComment struct {
	summary string
	id      string

	method string
	router string

	hasSuccess bool
	hasFailure bool
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
		if c == nil {
			return nil // do not fail next assertion for this route
		}

		assertConsistencyBetweenRouteIDAndSummary(t, *c)
		assertSuccessOrFailureDefined(t, *c)
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
		} else if strings.Contains(text, "@Summary ") {
			args := strings.SplitN(text, " ", 3)
			c.summary = args[2]
		} else if strings.Contains(text, "@ID ") {
			args := strings.SplitN(text, " ", 3)
			c.id = args[2]
		} else if strings.Contains(text, "@Success ") {
			c.hasSuccess = true
		} else if strings.Contains(text, "@Failure ") {
			c.hasFailure = true
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

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9-]+`)

func assertConsistencyBetweenRouteIDAndSummary(t *testing.T, comment swaggerComment) {
	exp := strings.ToLower(comment.summary)
	exp = strings.ReplaceAll(exp, " ", "-")
	exp = nonAlphanumericRegex.ReplaceAllString(exp, "")

	assert.Equal(t, exp, comment.id, "Router ID must match summary")
}

func assertSuccessOrFailureDefined(t *testing.T, comment swaggerComment) {
	assert.True(t, comment.hasSuccess || comment.hasFailure, "At least one @Success or @Failure annotation must be defined")
}
