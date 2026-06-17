// Package adapters maps a persisted guardrail adapter_type + JSON config to a
// concrete [guardrail.Guardrail]. It is the single registry both the management
// API (for validation) and the runtime loader (for construction) use, so a
// supported adapter is defined in exactly one place. It lives in its own package
// to avoid an import cycle: vendor adapters import the guardrail package, so the
// registry cannot live there.
package adapters

import (
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/aibridge/guardrail"
	"github.com/coder/coder/v2/aibridge/guardrail/bedrock"
	"github.com/coder/coder/v2/aibridge/guardrail/generic"
	"github.com/coder/coder/v2/aibridge/guardrail/presidio"
)

// Build constructs a guardrail from its stored adapter_type and config. The
// credential is the decrypted secret (empty for adapters that need none).
func Build(adapterType, name string, config []byte, credential string) (guardrail.Guardrail, error) {
	switch adapterType {
	case "presidio":
		return presidio.NewFromConfig(name, config)
	case "generic":
		return generic.NewFromConfig(name, config, credential)
	case "bedrock":
		return bedrock.NewFromConfig(name, config, credential)
	default:
		return nil, xerrors.Errorf("unknown guardrail adapter type %q", adapterType)
	}
}

// Validate checks that adapterType is known and config is well-formed, without
// performing any network I/O. It is the registration gate for the API.
func Validate(adapterType string, config []byte) error {
	// "validate" is a placeholder name; construction enforces config validity.
	_, err := Build(adapterType, "validate", config, "")
	return err
}
