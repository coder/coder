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
	summary  string
	id       string
	security string
	tags     string
	accept   string
	produce  string

	method string
	router string

	hasSuccess bool
	hasFailure bool

	parameters []parameter

	raw []*ast.Comment
}

type parameter struct {
	name string
	kind string
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
	c := SwaggerComment{
		raw:        commentGroup.List,
		parameters: []parameter{},
	}
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
		} else if strings.HasPrefix(text, "@Security ") {
			args := strings.SplitN(text, " ", 2)
			c.security = args[1]
		} else if strings.HasPrefix(text, "@Param ") {
			args := strings.SplitN(text, " ", 4)
			p := parameter{
				name: args[1],
				kind: args[2],
			}
			c.parameters = append(c.parameters, p)
		} else if strings.HasPrefix(text, "@Accept ") {
			args := strings.SplitN(text, " ", 2)
			c.accept = args[1]
		} else if strings.HasPrefix(text, "@Produce ") {
			args := strings.SplitN(text, " ", 2)
			c.produce = args[1]
		}
	}
	return c
}

func VerifySwaggerDefinitions(t *testing.T, router chi.Router, swaggerComments []SwaggerComment) {
	assertUniqueRoutes(t, swaggerComments)

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
			assertGoCommentFirst(t, *c)
			assertPathParametersDefined(t, *c)
			assertSecurityDefined(t, *c)
			assertRequestBody(t, *c)
		})
		return nil
	})
	require.NoError(t, err, "chi.Walk should not fail")
}

func assertUniqueRoutes(t *testing.T, comments []SwaggerComment) {
	m := map[string]struct{}{}

	for _, c := range comments {
		key := c.method + " " + c.router
		_, alreadyDefined := m[key]
		assert.False(t, alreadyDefined, "defined route must be unique (method: %s, route: %s)", c.method, c.router)
		if !alreadyDefined {
			m[key] = struct{}{}
		}
	}
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

func assertGoCommentFirst(t *testing.T, comment SwaggerComment) {
	var inSwaggerBlock bool

	for _, line := range comment.raw {
		text := strings.TrimSpace(line.Text)

		if inSwaggerBlock {
			if !strings.HasPrefix(text, "// @") {
				assert.Fail(t, "Go function comment must be placed before swagger comments")
				return
			}
		}
		if strings.HasPrefix(text, "// @Summary") {
			inSwaggerBlock = true
		}
	}
}

var urlParameterRegexp = regexp.MustCompile(`{[^{}]*}`)

func assertPathParametersDefined(t *testing.T, comment SwaggerComment) {
	matches := urlParameterRegexp.FindAllString(comment.router, -1)
	if matches == nil {
		return // router does not require any parameters
	}

	for _, m := range matches {
		var matched bool
		for _, p := range comment.parameters {
			if p.kind == "path" && "{"+p.name+"}" == m {
				matched = true
				break
			}
		}

		if !matched {
			assert.Failf(t, "Missing @Param annotation", "Path parameter: %s", m)
		}
	}
}

func assertSecurityDefined(t *testing.T, comment SwaggerComment) {
	if comment.router == "/updatecheck" ||
		comment.router == "/buildinfo" ||
		comment.router == "/" {
		return // endpoints do not require authorization
	}
	assert.Equal(t, "CoderSessionToken", comment.security, "@Security must be equal CoderSessionToken")
}

func assertRequestBody(t *testing.T, comment SwaggerComment) {
	var hasRequestBody bool
	for _, c := range comment.parameters {
		if c.name == "request" && c.kind == "body" ||
			c.name == "file" && c.kind == "formData" {
			hasRequestBody = true
			break
		}
	}

	var hasAccept bool
	if comment.accept != "" {
		hasAccept = true
	}

	if comment.method == "get" {
		assert.Empty(t, comment.accept, "GET route does not require the @Accept annotation")
		assert.False(t, hasRequestBody, "GET route does not require the request body")
	} else {
		assert.False(t, hasRequestBody && !hasAccept, "Route with the request body requires the @Accept annotation")
		assert.False(t, !hasRequestBody && hasAccept, "Route with @Accept annotation requires the request body or file formData parameter")
	}
}
