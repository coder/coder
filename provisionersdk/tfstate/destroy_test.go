package tfstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/tfstate"
)

func TestNetworkPolicyDestroyState(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, state string
		valid       bool
	}{
		{"Empty", `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","outputs":{},"resources":[]}`, true},
		{"EmptyManagedResource", `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[{"mode":"managed","instances":[]}]}`, true},
		{"DataOnly", `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[{"mode":"data","instances":[{}]}]}`, true},
		{"Missing", ``, false},
		{"Null", `null`, false},
		{"Object", `{}`, false},
		{"Preview", `{"format_version":"1.2","values":{"root_module":{}}}`, false},
		{"OldFormat", `{"version":3,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[]}`, false},
		{"MissingResources", `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage"}`, false},
		{"NullResources", `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":null}`, false},
		{"Managed", `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[{"mode":"managed","instances":[{}]}]}`, false},
		{"Deposed", `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[{"mode":"managed","instances":[{"deposed":"deadbeef"}]}]}`, false},
		{"UnknownMode", `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[{"mode":"other","instances":[]}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			for _, validate := range []func([]byte) error{tfstate.ValidateNetworkPolicyDestroyState, tfstate.ValidateDestroyState} {
				err := validate([]byte(tc.state))
				if tc.valid {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
			}
		})
	}
}
