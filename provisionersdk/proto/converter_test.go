package proto_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/terraform-provider-coder/v2/provider"
)

func TestProviderFormTypeEnum(t *testing.T) {
	t.Parallel()

	all := provider.ParameterFormTypes()
	for _, p := range all {
		t.Run(string(p), func(t *testing.T) {
			t.Parallel()
			_, err := proto.ProtoFormType(p)
			require.NoError(t, err, "proto form type should be valid, add it to the proto file")
		})
	}
}
