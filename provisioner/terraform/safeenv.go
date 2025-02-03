package terraform

import (
	"os"
	"strings"
)

// We must clean CODER_ environment variables to avoid accidentally passing in
// secrets like the Postgres connection string. See
// https://github.com/coder/coder/issues/4635.
//
// safeEnviron() is provided as an os.Environ() alternative that strips CODER_
// variables. As an additional precaution, we check a canary variable before
// provisioner exec.
//
// We cannot strip all CODER_ variables at exec because some are used to
// configure the provisioner.

const unsafeEnvCanary = "CODER_DONT_PASS"

func init() {
	_ = os.Setenv(unsafeEnvCanary, "true")
}

func envName(env string) string {
	parts := strings.SplitN(env, "=", 1)
	if len(parts) > 0 {
		return parts[0]
	}
	return ""
}

func envVar(env string) string {
	parts := strings.SplitN(env, "=", 1)
	if len(parts) > 0 {
		return parts[1]
	}
	return ""
}

func isCanarySet(env []string) bool {
	for _, e := range env {
		if envName(e) == unsafeEnvCanary {
			return true
		}
	}
	return false
}

// safeEnviron wraps os.Environ but removes CODER_ environment variables.
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
