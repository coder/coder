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

type SwaggerComment struct {
	summary string
	id      string
	tags    string

	method string
	router string

	hasSuccess bool
	hasFailure bool
}

func ParseSwaggerComments(dirs ...string) ([]SwaggerComment, error) {
	fileSet := token.NewFileSet()

	var swaggerComments []SwaggerComment
	for _, dir := range dirs {
		commentNodes, err := parser.ParseDir(fileSet, dir, nil, parser.ParseComments)
		if err != nil {
			return nil, xerrors.Errorf(`parser.ParseDir failed "%s": %w`, dir, err)
		}

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
	}
	return swaggerComments, nil
}

func parseSwaggerComment(commentGroup *ast.CommentGroup) SwaggerComment {
	var c SwaggerComment
	for _, line := range commentGroup.List {
		splitN := strings.SplitN(strings.TrimSpace(line.Text), " ", 2)
		if len(splitN) < 2 {
			continue // comment prefix without any content
		}
		text := splitN[1] // Skip the comment prefix (double-slash)

		if strings.HasPrefix(text, "@Router ") {
			args := strings.SplitN(text, " ", 3)
			c.router = args[1]
			c.method = args[2][1 : len(args[2])-1]
		} else if strings.HasPrefix(text, "@Summary ") {
			args := strings.SplitN(text, " ", 2)
			c.summary = args[1]
		} else if strings.HasPrefix(text, "@ID ") {
			args := strings.SplitN(text, " ", 2)
			c.id = args[1]
		} else if strings.HasPrefix(text, "@Success ") {
			c.hasSuccess = true
		} else if strings.HasPrefix(text, "@Failure ") {
			c.hasFailure = true
		} else if strings.HasPrefix(text, "@Tags ") {
			args := strings.SplitN(text, " ", 2)
			c.tags = args[1]
		}
	}
	return c
}

func VerifySwaggerDefinitions(t *testing.T, router chi.Router, swaggerComments []SwaggerComment) {
	err := chi.Walk(router, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		method = strings.ToLower(method)
		if route != "/" && strings.HasSuffix(route, "/") {
			route = route[:len(route)-1]
		}

		t.Run(method+" "+route, func(t *testing.T) {
			t.Parallel()

			c := findSwaggerCommentByMethodAndRoute(swaggerComments, method, route)
			assert.NotNil(t, c, "Missing @Router annotation")
			if c == nil {
				return // do not fail next assertion for this route
			}

			assertConsistencyBetweenRouteIDAndSummary(t, *c)
			assertSuccessOrFailureDefined(t, *c)
			assertRequiredAnnotations(t, *c)
		})
		return nil
	})
	require.NoError(t, err, "chi.Walk should not fail")
}

func findSwaggerCommentByMethodAndRoute(comments []SwaggerComment, method, route string) *SwaggerComment {
	for _, c := range comments {
		if c.method == method && c.router == route {
			return &c
		}
	}
	return nil
}

var nonAlphanumericRegex = regexp.MustCompile(`[^a-zA-Z0-9-]+`)

func assertConsistencyBetweenRouteIDAndSummary(t *testing.T, comment SwaggerComment) {
	exp := strings.ToLower(comment.summary)
	exp = strings.ReplaceAll(exp, " ", "-")
	exp = nonAlphanumericRegex.ReplaceAllString(exp, "")

	assert.Equal(t, exp, comment.id, "Router ID must match summary")
}

func assertSuccessOrFailureDefined(t *testing.T, comment SwaggerComment) {
	assert.True(t, comment.hasSuccess || comment.hasFailure, "At least one @Success or @Failure annotation must be defined")
}

func assertRequiredAnnotations(t *testing.T, comment SwaggerComment) {
	assert.NotEmpty(t, comment.id, "@ID must be defined")
	assert.NotEmpty(t, comment.summary, "@Summary must be defined")
	assert.NotEmpty(t, comment.tags, "@Tags must be defined")
}
