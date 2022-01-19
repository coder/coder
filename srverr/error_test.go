package srverr

import (
	"testing"

	"github.com/stretchr/testify/require"
	"golang.org/x/xerrors"
)

func TestErrorChain(t *testing.T) {
	t.Run("wrapping", func(t *testing.T) {
		err := xerrors.Errorf("im an error")
		err = Upgrade(err, ResourceNotFoundError{})
		err = xerrors.Errorf("wrapped http error: %w", err)

		var herr Error
		require.ErrorAs(t, err, &herr, "should find http error details")
	})
}
