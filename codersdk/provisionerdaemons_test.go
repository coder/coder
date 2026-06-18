package codersdk_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/codersdk"
)

func TestIsReservedProvisionerKey(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name     string
		id       uuid.UUID
		reserved bool
	}{
		{name: "BuiltIn", id: codersdk.ProvisionerKeyUUIDBuiltIn, reserved: true},
		{name: "UserAuth", id: codersdk.ProvisionerKeyUUIDUserAuth, reserved: true},
		{name: "PSK", id: codersdk.ProvisionerKeyUUIDPSK, reserved: true},
		{name: "Nil", id: uuid.Nil, reserved: false},
		{name: "Random", id: uuid.New(), reserved: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.reserved, codersdk.IsReservedProvisionerKey(tc.id))
		})
	}
}
