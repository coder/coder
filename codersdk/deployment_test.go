package codersdk_test

import (
	"testing"

	"github.com/coder/coder/codersdk"
)

func Test_DeploymentConfig_HasYAML(t *testing.T) {
	t.Parallel()

	// This test ensures that every deployment option has
	// a corresponding YAML name, unless explicitly excluded.

	excludes := map[string]struct{}{
		// These are used to configure YAML support itself, so
		// they make no sense within the YAML file.
		"Config Path":  {},
		"Write Config": {},
		// Dangerous values? Not sure we should help users
		// configure them.
		"DANGEROUS: Allow Path App Sharing":                {},
		"DANGEROUS: Allow Site Owners to Access Path Apps": {},
	}
	set := codersdk.NewDeploymentConfig().ConfigOptions()
	for _, opt := range set {
		if opt.YAML == "" && opt.Hidden {
			continue
		}

		if codersdk.IsSecretDeploymentOption(opt) {
			if opt.YAML != "" {
				// Secrets should not be written to YAML and instead should continue
				// to be read from the environment.
				t.Errorf("Option %q is a secret but has a YAML name", opt.Name)
				continue
			}
			continue
		}

		_, excluded := excludes[opt.Name]
		if opt.YAML == "" && !excluded {
			t.Errorf("Option %q has no YAML name", opt.Name)
		}
		if opt.YAML != "" && excluded {
			t.Errorf("Option %q is excluded but has a YAML name", opt.Name)
		}
		delete(excludes, opt.Name)
	}
	for opt := range excludes {
		t.Errorf("Excluded option %q is not in the deployment config. Remove it?", opt)
	}
}
