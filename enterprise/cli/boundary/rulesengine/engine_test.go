//nolint:paralleltest,testpackage
package rulesengine

import (
	"log/slog"
	"testing"
)

func TestEngineMatches(t *testing.T) {
	logger := slog.Default()
	engine := NewRuleEngine(nil, logger)

	tests := []struct {
		name     string
		rule     Rule
		method   string
		url      string
		expected bool
	}{
		// Method pattern tests
		{
			name: "method matches exact",
			rule: Rule{
				MethodPatterns: map[string]struct{}{"GET": {}},
			},
			method:   "GET",
			url:      "https://example.com/api",
			expected: true,
		},
		{
			name: "method does not match",
			rule: Rule{
				MethodPatterns: map[string]struct{}{"POST": {}},
			},
			method:   "GET",
			url:      "https://example.com/api",
			expected: false,
		},
		{
			name: "method wildcard matches any",
			rule: Rule{
				MethodPatterns: map[string]struct{}{"*": {}},
			},
			method:   "PUT",
			url:      "https://example.com/api",
			expected: true,
		},
		{
			name: "no method pattern allows all methods",
			rule: Rule{
				HostPattern: []string{"example", "com"},
			},
			method:   "DELETE",
			url:      "https://example.com/api",
			expected: true,
		},

		// Host pattern tests
		{
			name: "host matches exact",
			rule: Rule{
				HostPattern: []string{"example", "com"},
			},
			method:   "GET",
			url:      "https://example.com/api",
			expected: true,
		},
		{
			name: "host does not match",
			rule: Rule{
				HostPattern: []string{"example", "org"},
			},
			method:   "GET",
			url:      "https://example.com/api",
			expected: false,
		},
		{
			name: "subdomain matches",
			rule: Rule{
				HostPattern: []string{"example", "com"},
			},
			method:   "GET",
			url:      "https://api.example.com/users",
			expected: true,
		},
		{
			name: "host pattern too long",
			rule: Rule{
				HostPattern: []string{"v1", "api", "example", "com"},
			},
			method:   "GET",
			url:      "https://api.example.com/users",
			expected: false,
		},
		{
			name: "host wildcard matches",
			rule: Rule{
				HostPattern: []string{"*", "com"},
			},
			method:   "GET",
			url:      "https://test.com/api",
			expected: true,
		},
		{
			name: "multiple host wildcards",
			rule: Rule{
				HostPattern: []string{"*", "*"},
			},
			method:   "GET",
			url:      "https://api.example.com/users",
			expected: true,
		},

		// Path pattern tests
		{
			name: "path matches exact",
			rule: Rule{
				PathPattern: [][]string{{"api", "users"}},
			},
			method:   "GET",
			url:      "https://example.com/api/users",
			expected: true,
		},
		{
			name: "path does not match",
			rule: Rule{
				PathPattern: [][]string{{"api", "posts"}},
			},
			method:   "GET",
			url:      "https://example.com/api/users",
			expected: false,
		},
		{
			name: "subpath does not implicitly match",
			rule: Rule{
				PathPattern: [][]string{{"api"}},
			},
			method:   "GET",
			url:      "https://example.com/api/users/123",
			expected: false,
		},
		{
			name: "asterisk matches in path",
			rule: Rule{
				PathPattern: [][]string{{"api", "*"}},
			},
			method:   "GET",
			url:      "https://example.com/api/users/123",
			expected: true,
		},
		{
			name: "one asterisk at end matches any number of trailing segments",
			rule: Rule{
				PathPattern: [][]string{{"api", "*"}},
			},
			method:   "GET",
			url:      "https://example.com/api/foo/bar/baz",
			expected: true,
		},
		{
			name: "asterisk in middle of path only matches one segment",
			rule: Rule{
				PathPattern: [][]string{{"api", "*", "foo"}},
			},
			method:   "GET",
			url:      "https://example.com/api/users/admin/foo",
			expected: false,
		},
		{
			name: "path pattern too long",
			rule: Rule{
				PathPattern: [][]string{{"api", "v1", "users", "profile"}},
			},
			method:   "GET",
			url:      "https://example.com/api/v1/users",
			expected: false,
		},
		{
			name: "path wildcard matches",
			rule: Rule{
				PathPattern: [][]string{{"api", "*", "profile"}},
			},
			method:   "GET",
			url:      "https://example.com/api/users/profile",
			expected: true,
		},
		{
			name: "multiple path wildcards",
			rule: Rule{
				PathPattern: [][]string{{"*", "*"}},
			},
			method:   "GET",
			url:      "https://example.com/api/users/123",
			expected: true,
		},

		// Combined pattern tests
		{
			name: "all patterns match",
			rule: Rule{
				MethodPatterns: map[string]struct{}{"POST": {}},
				HostPattern:    []string{"api", "com"},
				PathPattern:    [][]string{{"users"}},
			},
			method:   "POST",
			url:      "https://api.com/users",
			expected: true,
		},
		{
			name: "method fails combined test",
			rule: Rule{
				MethodPatterns: map[string]struct{}{"POST": {}},
				HostPattern:    []string{"api", "com"},
				PathPattern:    [][]string{{"users"}},
			},
			method:   "GET",
			url:      "https://api.com/users",
			expected: false,
		},
		{
			name: "host fails combined test",
			rule: Rule{
				MethodPatterns: map[string]struct{}{"POST": {}},
				HostPattern:    []string{"api", "org"},
				PathPattern:    [][]string{{"users"}},
			},
			method:   "POST",
			url:      "https://api.com/users",
			expected: false,
		},
		{
			name: "path fails combined test",
			rule: Rule{
				MethodPatterns: map[string]struct{}{"POST": {}},
				HostPattern:    []string{"api", "com"},
				PathPattern:    [][]string{{"posts"}},
			},
			method:   "POST",
			url:      "https://api.com/users",
			expected: false,
		},
		{
			name: "all wildcards match",
			rule: Rule{
				MethodPatterns: map[string]struct{}{"*": {}},
				HostPattern:    []string{"*", "*"},
				PathPattern:    [][]string{{"*", "*"}},
			},
			method:   "PATCH",
			url:      "https://test.example.com/api/users/123",
			expected: true,
		},

		// Edge cases
		{
			name:     "empty rule matches everything",
			rule:     Rule{},
			method:   "GET",
			url:      "https://example.com/api/users",
			expected: true,
		},
		{
			name: "invalid URL",
			rule: Rule{
				HostPattern: []string{"example", "com"},
			},
			method:   "GET",
			url:      "not-a-valid-url",
			expected: false,
		},
		{
			name: "root path",
			rule: Rule{
				PathPattern: [][]string{{}},
			},
			method:   "GET",
			url:      "https://example.com/",
			expected: true,
		},
		{
			name: "localhost host",
			rule: Rule{
				HostPattern: []string{"localhost"},
			},
			method:   "GET",
			url:      "http://localhost:8080/api",
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := engine.matches(tt.rule, tt.method, tt.url)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
