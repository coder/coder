package chatfiles_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatfiles"
)

func TestPDFPreflightHelpers(t *testing.T) {
	t.Parallel()

	require.True(t, chatfiles.IsPDF([]byte("%PDF-1.7\n")))
	require.False(t, chatfiles.IsPDF(nil))
	require.False(t, chatfiles.IsPDF([]byte("%PDF")))
	require.False(t, chatfiles.IsPDF([]byte("hello")))

	require.True(t, chatfiles.IsEncryptedPDF([]byte("%PDF-1.7\ntrailer << /Encrypt 5 0 R >>")))
	require.True(t, chatfiles.IsEncryptedPDF([]byte("%PDF-1.7\ntrailer << /Encrypt << /Filter /Standard >> >>")))
	// /U and /O entries without /Encrypt must not flag a valid PDF.
	require.False(t, chatfiles.IsEncryptedPDF([]byte("%PDF-1.7\n/U (foo) /O (bar)")))
	require.False(t, chatfiles.IsEncryptedPDF([]byte("%PDF-1.7\n<< /Type /Page >>")))
	// Incidental "/Encrypt" prose in document text or metadata must not flag.
	require.False(t, chatfiles.IsEncryptedPDF([]byte("%PDF-1.7\n(set the /Encrypt entry in the trailer)")))
	require.False(t, chatfiles.IsEncryptedPDF([]byte("%PDF-1.7\n(see /Encrypt 5 0 Reference)")))

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
