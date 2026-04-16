package cliui

import (
	"bufio"
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSecretRunes(t *testing.T) {
	t.Parallel()

	t.Run("PlainInput", func(t *testing.T) {
		t.Parallel()
		input := "hello\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "hello", result)
		assert.Equal(t, "*****\r\n", out.String())
	})

	t.Run("BracketedPaste", func(t *testing.T) {
		t.Parallel()
		// Simulates: ESC[200~ token123 ESC[201~ Enter
		input := "\x1b[200~token123\x1b[201~\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "token123", result)
		assert.Equal(t, "********\r\n", out.String())
	})

	t.Run("ArrowKeysIgnored", func(t *testing.T) {
		t.Parallel()
		// Type "ab", press Up arrow (ESC[A), type "cd", Enter.
		input := "ab\x1b[Acd\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "abcd", result)
		assert.Equal(t, "****\r\n", out.String())
	})

	t.Run("FunctionKeyIgnored", func(t *testing.T) {
		t.Parallel()
		// Type "x", press F5 (ESC[15~), type "y", Enter.
		input := "x\x1b[15~y\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "xy", result)
		assert.Equal(t, "**\r\n", out.String())
	})

	t.Run("EscOSequenceIgnored", func(t *testing.T) {
		t.Parallel()
		// ESC O P (F1 in application-mode) between typed chars.
		input := "a\x1bOPb\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		// The full SS3 sequence (ESC O P) should be discarded.
		assert.Equal(t, "ab", result)
		assert.Equal(t, "**\r\n", out.String())
	})

	t.Run("Backspace", func(t *testing.T) {
		t.Parallel()
		// Type "abc", backspace, type "d", Enter.
		input := "abc\x7fd\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "abd", result)
	})

	t.Run("BackspaceBS", func(t *testing.T) {
		t.Parallel()
		// Same but with \b (0x08) instead of DEL (0x7F).
		input := "abc\bd\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "abd", result)
	})

	t.Run("CtrlC", func(t *testing.T) {
		t.Parallel()
		input := "abc\x03"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		_, err := readSecretRunes(reader, &out)
		assert.ErrorIs(t, err, ErrCanceled)
	})

	t.Run("UTF8", func(t *testing.T) {
		t.Parallel()
		input := "和製漢字\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "和製漢字", result)
		assert.Equal(t, "****\r\n", out.String())
	})

	t.Run("NewlineTerminator", func(t *testing.T) {
		t.Parallel()
		input := "test\n"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "test", result)
	})

	t.Run("ControlCharsIgnored", func(t *testing.T) {
		t.Parallel()
		// Tab (0x09) and other control chars should be silently
		// dropped without being appended to the result.
		input := "a\tb\x01c\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "abc", result)
		assert.Equal(t, "***\r\n", out.String())
	})

	t.Run("EmptyInput", func(t *testing.T) {
		t.Parallel()
		input := "\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "", result)
	})

	t.Run("EOF", func(t *testing.T) {
		t.Parallel()
		// EOF without a newline should return an error.
		input := "abc"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		_, err := readSecretRunes(reader, &out)
		assert.ErrorIs(t, err, io.EOF)
	})

	t.Run("MultipleBracketedPastes", func(t *testing.T) {
		t.Parallel()
		// Two bracketed pastes back to back (unlikely but
		// defensive).
		input := "\x1b[200~abc\x1b[201~\x1b[200~def\x1b[201~\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "abcdef", result)
	})

	t.Run("BareEsc", func(t *testing.T) {
		t.Parallel()
		// A bare ESC followed by a normal printable character
		// (not '[' or 'O') should discard only the ESC and keep
		// the following character.
		input := "a\x1bxb\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "axb", result)
		assert.Equal(t, "***\r\n", out.String())
	})

	t.Run("BackspaceOnEmpty", func(t *testing.T) {
		t.Parallel()
		// Backspace when buffer is empty should be a no-op.
		input := "\x7f\x7fabc\r"
		reader := bufio.NewReader(strings.NewReader(input))
		var out bytes.Buffer

		result, err := readSecretRunes(reader, &out)
		require.NoError(t, err)
		assert.Equal(t, "abc", result)
	})
}
