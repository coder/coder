// This file tests an internal function.
//nolint:testpackage
package coderd

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_websocketCloseMsg(t *testing.T) {
	t.Parallel()

	t.Run("TruncateSingleByteCharacters", func(t *testing.T) {
		t.Parallel()

		msg := strings.Repeat("d", 255)
		trunc := fmtWebsocketCloseMsg(msg)
		assert.LessOrEqual(t, len(trunc), 123)
	})

	t.Run("TruncateMultiByteCharacters", func(t *testing.T) {
		t.Parallel()

		msg := strings.Repeat("こんにちは", 10)
		trunc := fmtWebsocketCloseMsg(msg)
		assert.LessOrEqual(t, len(trunc), 123)
	})
}
