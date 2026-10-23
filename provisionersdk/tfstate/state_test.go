package tfstate_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/tfstate"
)

func TestValidateState(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, state string
		valid       bool
	}{
		{"EmptyResources", `{"version":4,"terraform_version":"1.15.0","serial":0,"lineage":"lineage","outputs":{},"resources":[]}`, true},
		{"Managed", `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[{"mode":"managed","instances":[{}]}]}`, true},
		{"Deposed", `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[{"mode":"managed","instances":[{"deposed":"deadbeef"}]}]}`, true},
		{"Data", `{"version":4,"terraform_version":"1.15.0","serial":2,"lineage":"lineage","resources":[{"mode":"data","instances":[{}]}]}`, true},
		{"Null", `null`, false},
		{"Missing", ``, false},
		{"Object", `{}`, false},
		{"Preview", `{"format_version":"1.2","values":{"root_module":{}}}`, false},
		{"NegativeSerial", `{"version":4,"terraform_version":"1.15.0","serial":-1,"lineage":"lineage","resources":[]}`, false},
		{"NullResources", `{"version":4,"terraform_version":"1.15.0","serial":0,"lineage":"lineage","resources":null}`, false},
		{"InvalidResources", `{"version":4,"terraform_version":"1.15.0","serial":0,"lineage":"lineage","resources":{}}`, false},
		{"UnknownMode", `{"version":4,"terraform_version":"1.15.0","serial":0,"lineage":"lineage","resources":[{"mode":"unknown"}]}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tfstate.ValidateState([]byte(tc.state))
			if tc.valid {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}
