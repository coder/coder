package rbac_test
import (
	"fmt"
	"errors"
	"testing"
	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/stretchr/testify/require"
)
func TestIsUnauthorizedError(t *testing.T) {
	t.Parallel()
	t.Run("NotWrapped", func(t *testing.T) {
		t.Parallel()
		errFunc := func() error {
			return rbac.UnauthorizedError{}
		}
		err := errFunc()
		require.True(t, rbac.IsUnauthorizedError(err))
	})
	t.Run("Wrapped", func(t *testing.T) {
		t.Parallel()
		errFunc := func() error {
			return fmt.Errorf("test error: %w", rbac.UnauthorizedError{})
		}
		err := errFunc()
		require.True(t, rbac.IsUnauthorizedError(err))
	})
}
