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

// safeEnvironValue returns the value of the named variable in the given
// `KEY=VALUE` environment slice, or an empty string if it is not present.
func safeEnvironValue(env []string, name string) string {
	prefix := name + "="
	for _, e := range env {
		if strings.HasPrefix(e, prefix) {
			return strings.TrimPrefix(e, prefix)
		}
	}
	return ""
}

const (
	awsSDKUserAgentEnvKey = "AWS_SDK_UA_APP_ID"
	// awsSDKUserAgentCoder is Coder's AWS Partner Revenue Measurement
	// User-Agent string. The `APN_1.1/pc_<product-code>$` format and the
	// space-delimited append behavior below follow AWS's guidance:
	// https://docs.aws.amazon.com/PRM/latest/aws-prm-onboarding-guide/automated-user-agent.html
	awsSDKUserAgentCoder = "APN_1.1/pc_cdfmjwn8i6u8l9fwz8h82e4w3$"
)

// awsSDKUserAgentEnv returns the AWS_SDK_UA_APP_ID value to pass to the
// Terraform subprocess. If the caller's environment already configures an
// Application ID (e.g. an operator who is also an AWS Partner and wants
// their own revenue attribution), Coder's value is appended with a space
// delimiter so both attributions are preserved. Otherwise Coder's value is
// used on its own.
//
// See: https://docs.aws.amazon.com/PRM/latest/aws-prm-onboarding-guide/automated-user-agent.html
func awsSDKUserAgentEnv(existing string) string {
	if existing == "" {
		return awsSDKUserAgentEnvKey + "=" + awsSDKUserAgentCoder
	}
	return awsSDKUserAgentEnvKey + "=" + existing + " " + awsSDKUserAgentCoder
}
