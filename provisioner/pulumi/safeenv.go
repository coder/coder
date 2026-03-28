package pulumi

import (
	"os"
	"strings"
)

const unsafeEnvCanary = "CODER_DONT_PASS"

func init() {
	_ = os.Setenv(unsafeEnvCanary, "true")
}

func envName(env string) string {
	parts := strings.SplitN(env, "=", 2)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

var _ = envName

func isCanarySet(env []string) bool {
	for _, e := range env {
		if envName(e) == unsafeEnvCanary {
			return true
		}
	}
	return false
}

func safeEnviron() []string {
	env := os.Environ()
	strippedEnv := make([]string, 0, len(env))
	for _, e := range env {
		name := envName(e)
		if strings.HasPrefix(name, "CODER_") {
			continue
		}
		strippedEnv = append(strippedEnv, e)
	}
	return strippedEnv
}
