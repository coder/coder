package coderd_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/goleak"

	"github.com/coder/coder/coderd"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestFmtWebsocketCloseMsg(t *testing.T) {
	t.Parallel()

	t.Run("TruncateSingleByteCharacters", func(t *testing.T) {
		t.Parallel()

		msg := strings.Repeat("d", 255)
		trunc := coderd.FmtWebsocketCloseMsg(msg)
		assert.LessOrEqual(t, len(trunc), 123)
	})

	t.Run("TruncateMultiByteCharacters", func(t *testing.T) {
		t.Parallel()

		msg := strings.Repeat("こんにちは", 10)
		trunc := coderd.FmtWebsocketCloseMsg(msg)
		assert.LessOrEqual(t, len(trunc), 123)
	})
}
