package chattest_test

import (
	"os"
	"strings"
	"testing"
)

type providerRunMode struct {
	Name string
	Live bool
}

func providerModes(t *testing.T, apiKeyEnv string) []providerRunMode {
	t.Helper()

	modes := []providerRunMode{{Name: "Mock", Live: false}}
	if strings.TrimSpace(os.Getenv(apiKeyEnv)) == "" {
		t.Logf("Skipping Live mode: %s is not set.", apiKeyEnv)
		return modes
	}

	return append(modes, providerRunMode{Name: "Live", Live: true})
}

func envOrDefault(envKey, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(envKey)); value != "" {
		return value
	}
	return fallback
}
