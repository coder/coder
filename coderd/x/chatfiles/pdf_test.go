package chatfiles_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatfiles"
)

func TestPDFPreflightHelpers(t *testing.T) {
	t.Parallel()

	require.True(t, chatfiles.IsPDF([]byte("%PDF-1.7\n")))
	require.False(t, chatfiles.IsPDF([]byte("%PDF")))
	require.False(t, chatfiles.IsPDF([]byte("hello")))

	count, ok := chatfiles.ApproxPDFPageCount([]byte("%PDF-1.7\n<< /Type /Page >>\n<< /Type\t/Page >>"))
	require.True(t, ok)
	require.Equal(t, 2, count)

	count, ok = chatfiles.ApproxPDFPageCount([]byte("%PDF-1.7\n<< /Type /Pages /Count 2 >>"))
	require.False(t, ok)
	require.Zero(t, count)

	count, ok = chatfiles.ApproxPDFPageCount([]byte("%PDF-1.7\nxref\n0 0"))
	require.False(t, ok)
	require.Zero(t, count)
}
