package main

import (
	"strings"
	"testing"
)

func TestRewritePathKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want string
	}{
		// Root mounted paths must be left alone.
		{
			name: "scim path stays at root",
			in:   "/scim/v2/Users",
			want: "/scim/v2/Users",
		},
		{
			name: "scim with id stays at root",
			in:   "/scim/v2/Users/{id}",
			want: "/scim/v2/Users/{id}",
		},
		{
			name: "oauth2 path stays at root",
			in:   "/oauth2/tokens",
			want: "/oauth2/tokens",
		},
		{
			name: "well-known path stays at root",
			in:   "/.well-known/oauth-authorization-server",
			want: "/.well-known/oauth-authorization-server",
		},
		{
			name: "api/experimental path stays at root",
			in:   "/api/experimental/watch-all-workspacebuilds",
			want: "/api/experimental/watch-all-workspacebuilds",
		},
		// Annotations using `/experimental/...` are canonicalized to
		// `/api/experimental/...`.
		{
			name: "experimental path canonicalized to api/experimental",
			in:   "/experimental/chats/config/retention-days",
			want: "/api/experimental/chats/config/retention-days",
		},
		// Everything else gets the /api/v2 prefix.
		{
			name: "regular endpoint gets api/v2 prefix",
			in:   "/users",
			want: "/api/v2/users",
		},
		{
			name: "endpoint with template parameter gets api/v2 prefix",
			in:   "/users/{user}",
			want: "/api/v2/users/{user}",
		},
		{
			name: "oauth2-provider is NOT root mounted",
			in:   "/oauth2-provider/apps",
			want: "/api/v2/oauth2-provider/apps",
		},
		{
			name: "root path becomes /api/v2/",
			in:   "/",
			want: "/api/v2/",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := rewritePathKey(tc.in)
			if got != tc.want {
				t.Errorf("rewritePathKey(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestRewriteSpecBytesPaths(t *testing.T) {
	t.Parallel()

	const input = `{
    "swagger": "2.0",
    "basePath": "/api/v2",
    "paths": {
        "/": {
            "get": {}
        },
        "/users": {
            "get": {}
        },
        "/scim/v2/Users": {
            "get": {}
        },
        "/.well-known/oauth-authorization-server": {
            "get": {}
        },
        "/oauth2/tokens": {
            "post": {}
        },
        "/experimental/chats/config/retention-days": {
            "get": {}
        }
    }
}
`

	out, err := rewriteSpecBytes([]byte(input), "swagger.json")
	if err != nil {
		t.Fatalf("rewriteSpecBytes: %v", err)
	}
	got := string(out)

	wantContains := []string{
		`"basePath": "/",`,
		`"/api/v2/": {`,
		`"/api/v2/users": {`,
		`"/scim/v2/Users": {`,
		`"/.well-known/oauth-authorization-server": {`,
		`"/oauth2/tokens": {`,
		`"/api/experimental/chats/config/retention-days": {`,
	}
	for _, want := range wantContains {
		if !strings.Contains(got, want) {
			t.Errorf("rewritten spec missing %q.\nfull output:\n%s", want, got)
		}
	}

	wantMissing := []string{
		`"basePath": "/api/v2",`,
		`"/api/v2/scim/`,
		`"/api/v2/oauth2/`,
		`"/api/v2/.well-known/`,
		`"/api/v2/api/experimental/`,
		`"/api/v2/experimental/`,
		`"/experimental/chats/`,
	}
	for _, missing := range wantMissing {
		if strings.Contains(got, missing) {
			t.Errorf("rewritten spec unexpectedly contains %q.\nfull output:\n%s", missing, got)
		}
	}
}

func TestRewriteSpecBytesDocsGoBasePath(t *testing.T) {
	t.Parallel()

	// docs.go has both the docTemplate string and the SwaggerInfo
	// struct's BasePath field.  We need the struct's BasePath field
	// to be rewritten so the runtime SwaggerInfo reflects the new
	// basePath; the docTemplate `"basePath": "{{.BasePath}}"` token
	// is filled in at runtime from SwaggerInfo and must not be
	// touched directly.
	const input = "package apidoc\n" +
		"const docTemplate = `{\n" +
		"    \"basePath\": \"{{.BasePath}}\",\n" +
		"    \"paths\": {\n" +
		"        \"/users\": {}\n" +
		"    }\n" +
		"}`\n" +
		"var SwaggerInfo = &swag.Spec{\n" +
		"\tBasePath:         \"/api/v2\",\n" +
		"}\n"

	out, err := rewriteSpecBytes([]byte(input), "docs.go")
	if err != nil {
		t.Fatalf("rewriteSpecBytes: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, `BasePath:         "/",`) {
		t.Errorf("expected SwaggerInfo BasePath to be rewritten, got:\n%s", got)
	}
	if !strings.Contains(got, `"basePath": "{{.BasePath}}"`) {
		t.Errorf("expected docTemplate {{.BasePath}} token to be preserved, got:\n%s", got)
	}
	if !strings.Contains(got, `"/api/v2/users": {}`) {
		t.Errorf("expected path keys to be rewritten in docTemplate, got:\n%s", got)
	}
}

func TestRewriteSpecBytesRejectsUnexpectedBasePath(t *testing.T) {
	t.Parallel()

	const input = `{
    "swagger": "2.0",
    "basePath": "/somethingelse",
    "paths": {
        "/users": {}
    }
}
`
	_, err := rewriteSpecBytes([]byte(input), "swagger.json")
	if err == nil {
		t.Fatal("expected an error when basePath is not /api/v2")
	}
}
