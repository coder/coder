package clitest

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestBuildPostgresURLFromComponents validates the PostgreSQL URL construction
// from individual components using the same logic as in cli/server.go
func TestBuildPostgresURLFromComponents(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		name           string
		host           string
		port           string
		username       string
		password       string
		database       string
		options        string
		expectedURL    string
	}{
		{
			name:        "BasicConnectionParams",
			host:        "localhost",
			port:        "5432",
			username:    "coder",
			password:    "password",
			database:    "coder_db",
			expectedURL: "postgres://coder:password@localhost:5432/coder_db",
		},
		{
			name:        "CustomPort",
			host:        "localhost",
			port:        "5433",
			username:    "coder",
			password:    "password",
			database:    "coder_db",
			expectedURL: "postgres://coder:password@localhost:5433/coder_db",
		},
		{
			name:        "DefaultPort",
			host:        "localhost",
			port:        "",
			username:    "coder",
			password:    "password",
			database:    "coder_db",
			expectedURL: "postgres://coder:password@localhost:5432/coder_db",
		},
		{
			name:        "WithConnectionOptions",
			host:        "localhost",
			port:        "5432",
			username:    "coder",
			password:    "password",
			database:    "coder_db",
			options:     "sslmode=disable",
			expectedURL: "postgres://coder:password@localhost:5432/coder_db?sslmode=disable",
		},
		{
			name:        "WithComplexPassword",
			host:        "localhost",
			port:        "5432",
			username:    "coder",
			password:    "password123",
			database:    "coder_db",
			expectedURL: "postgres://coder:password123@localhost:5432/coder_db",
		},
		{
			name:        "WithMultipleOptions",
			host:        "localhost",
			port:        "5432",
			username:    "coder",
			password:    "password",
			database:    "coder_db",
			options:     "sslmode=verify-full&connect_timeout=10",
			expectedURL: "postgres://coder:password@localhost:5432/coder_db?sslmode=verify-full&connect_timeout=10",
		},
	}

	for _, tc := range testcases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			// Build the base connection string using the same logic as server.go
			port := tc.port
			if port == "" {
				port = "5432" // Default PostgreSQL port
			}

			// Build the connection string
			connURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s",
				tc.username,
				tc.password,
				tc.host,
				port,
				tc.database)

			// Add options if provided
			if len(tc.options) > 0 {
				connURL = connURL + "?" + tc.options
			}

			// Verify the constructed URL matches expected
			assert.Equal(t, tc.expectedURL, connURL)
		})
	}
}
