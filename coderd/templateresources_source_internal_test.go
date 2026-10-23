package coderd

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateTemplateResourceSource(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		source string
		valid  bool
	}{
		{"ordinary", "resource \"terraform_data\" \"example\" { input = { nested = [true, 443] } }\n", true},
		{"provider", "terraform {\n required_providers {\n random = { source = \"hashicorp/random\" }\n }\n}\n", true},
		{"remote_backend", "terraform {\n backend \"s3\" { bucket = \"example\" }\n}\n", false},
		{"local_backend", "terraform {\n backend \"local\" { path = \"elsewhere.tfstate\" }\n}\n", false},
		{"cloud", "terraform {\n cloud { organization = \"example\" }\n}\n", false},
		{"malformed", "resource {", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := validateTemplateResourceSource(tc.source)
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
