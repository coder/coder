package codersdk_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/coderd/database"
	"github.com/coder/coder/codersdk"
)

func TestAuditDBEnumsCovered(t *testing.T) {
	t.Parallel()

	//nolint: gocritic
	dbTypes := database.AllResourceTypeValues()
	for _, ty := range dbTypes {
		str := codersdk.ResourceType(ty).FriendlyString()
		require.NotEqualf(t, "unknown", str, "ResourceType %q not covered by codersdk.ResourceType", ty)
	}
}
