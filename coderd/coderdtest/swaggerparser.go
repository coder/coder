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

	successes []response
	failures  []response

	parameters []parameter

	raw []*ast.Comment
}

type parameter struct {
	name string
	kind string
}

type response struct {
	status string
	kind   string // {object} or {array}
	model  string
}

func ParseSwaggerComments(dirs ...string) ([]SwaggerComment, error) {
	fileSet := token.NewFileSet()

	var swaggerComments []SwaggerComment
	for _, dir := range dirs {
		nodes, err := parser.ParseDir(fileSet, dir, nil, parser.ParseComments)
		if err != nil {
			return nil, xerrors.Errorf(`parser.ParseDir failed for "%s": %w`, dir, err)
		}

		for _, node := range nodes {
			ast.Inspect(node, func(n ast.Node) bool {
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
		successes:  []response{},
		failures:   []response{},
	}
	for _, line := range commentGroup.List {
		// @<annotationName> [args...]
		splitN := strings.SplitN(strings.TrimSpace(line.Text), " ", 3)
		if len(splitN) < 2 {
			continue // comment prefix without any content
		}

		if !strings.HasPrefix(splitN[1], "@") {
			continue // not a swagger annotation
		}

		annotationName := splitN[1]
		annotationArgs := splitN[2]
		args := strings.Split(splitN[2], " ")

		switch annotationName {
		case "@Router":
			c.router = args[0]
			c.method = args[1][1 : len(args[1])-1]
		case "@Success", "@Failure":
			var r response
			if len(args) > 0 {
				r.status = args[0]
			}
			if len(args) > 1 {
				r.kind = args[1]
			}
			if len(args) > 2 {
				r.model = args[2]
			}

			if annotationName == "@Success" {
				c.successes = append(c.successes, r)
			} else if annotationName == "@Failure" {
				c.failures = append(c.failures, r)
			}
		case "@Param":
			p := parameter{
				name: args[0],
				kind: args[1],
			}
			c.parameters = append(c.parameters, p)
		case "@Summary":
			c.summary = annotationArgs
		case "@ID":
			c.id = annotationArgs
		case "@Tags":
			c.tags = annotationArgs
		case "@Security":
			c.security = annotationArgs
		case "@Accept":
			c.accept = annotationArgs
		case "@Produce":
			c.produce = annotationArgs
		}
	}
	return c
}

func VerifySwaggerDefinitions(t *testing.T, router chi.Router, swaggerComments []SwaggerComment) {
	assertUniqueRoutes(t, swaggerComments)
	assertSingleAnnotations(t, swaggerComments)

	err := chi.Walk(router, func(method, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		method = strings.ToLower(method)
		if route != "/" && strings.HasSuffix(route, "/") {
			route = route[:len(route)-1]
		}

		t.Run(method+" "+route, func(t *testing.T) {
			t.Parallel()

			// This route is for compatibility purposes and is not documented.
			if route == "/workspaceagents/me/metadata" {
				return
			}

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
			assertAccept(t, *c)
			assertProduce(t, *c)
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

var uniqueAnnotations = []string{"@ID", "@Summary", "@Tags", "@Router"}

func assertSingleAnnotations(t *testing.T, comments []SwaggerComment) {
	for _, comment := range comments {
		counters := map[string]int{}

		for _, line := range comment.raw {
			splitN := strings.SplitN(strings.TrimSpace(line.Text), " ", 3)
			if len(splitN) < 2 {
				continue // comment prefix without any content
			}

			if !strings.HasPrefix(splitN[1], "@") {
				continue // not a swagger annotation
			}

			annotation := splitN[1]
			if _, ok := counters[annotation]; !ok {
				counters[annotation] = 0
			}
			counters[annotation]++
		}

		for _, annotation := range uniqueAnnotations {
			v := counters[annotation]
			assert.Equal(t, 1, v, "%s annotation for route %s must be defined only once", annotation, comment.router)
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
	assert.True(t, len(comment.successes) > 0 || len(comment.failures) > 0, "At least one @Success or @Failure annotation must be defined")
}

func assertRequiredAnnotations(t *testing.T, comment SwaggerComment) {
	assert.NotEmpty(t, comment.id, "@ID must be defined")
	assert.NotEmpty(t, comment.summary, "@Summary must be defined")
	assert.NotEmpty(t, comment.tags, "@Tags must be defined")
	assert.NotEmpty(t, comment.router, "@Router must be defined")
}

func assertGoCommentFirst(t *testing.T, comment SwaggerComment) {
	var inSwaggerBlock bool

	for _, line := range comment.raw {
		text := strings.TrimSpace(line.Text)

		if inSwaggerBlock {
			if !strings.HasPrefix(text, "// @") && !strings.HasPrefix(text, "// nolint:") {
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
		comment.router == "/" ||
		comment.router == "/users/login" {
		return // endpoints do not require authorization
	}
	assert.Equal(t, "CoderSessionToken", comment.security, "@Security must be equal CoderSessionToken")
}

func assertAccept(t *testing.T, comment SwaggerComment) {
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

var allowedProduceTypes = []string{"json", "text/event-stream", "text/html"}

func assertProduce(t *testing.T, comment SwaggerComment) {
	var hasResponseModel bool
	for _, r := range comment.successes {
		if r.model != "" {
			hasResponseModel = true
			break
		}
	}

	if hasResponseModel {
		assert.True(t, comment.produce != "", "Route must have @Produce annotation as it responds with a model structure")
		assert.Contains(t, allowedProduceTypes, comment.produce, "@Produce value is limited to specific types: %s", strings.Join(allowedProduceTypes, ","))
	} else {
		if (comment.router == "/workspaceagents/me/app-health" && comment.method == "post") ||
			(comment.router == "/workspaceagents/me/startup" && comment.method == "post") ||
			(comment.router == "/workspaceagents/me/startup/logs" && comment.method == "patch") ||
			(comment.router == "/licenses/{id}" && comment.method == "delete") ||
			(comment.router == "/debug/coordinator" && comment.method == "get") ||
			(comment.router == "/debug/tailnet" && comment.method == "get") {
			return // Exception: HTTP 200 is returned without response entity
		}

		assert.Truef(t, comment.produce == "", "Response model is undefined, so we can't predict the content type: %v", comment)
	}
}
