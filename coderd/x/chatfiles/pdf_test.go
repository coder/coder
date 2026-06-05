package chatfiles_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatfiles"
)

func TestIsPDF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "ValidMinimalHeader",
			data: []byte("%PDF-1.7\n"),
			want: true,
		},
		{
			name: "Empty",
			data: nil,
			want: false,
		},
		{
			name: "Short",
			data: []byte("%PDF"),
			want: false,
		},
		{
			name: "NonPDF",
			data: []byte("hello"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, chatfiles.IsPDF(tt.data))
		})
	}
}

func TestIsEncryptedPDF(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data []byte
		want bool
	}{
		{
			name: "EncryptMarker",
			data: []byte("%PDF-1.7\ntrailer << /Encrypt 5 0 R >>"),
			want: true,
		},
		{
			name: "UserPasswordMarker",
			data: []byte("%PDF-1.7\n<< /U (secret) >>"),
			want: true,
		},
		{
			name: "OwnerPasswordMarker",
			data: []byte("%PDF-1.7\n<< /O (secret) >>"),
			want: true,
		},
		{
			name: "Unencrypted",
			data: []byte("%PDF-1.7\n1 0 obj << /Type /Page >> endobj"),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			require.Equal(t, tt.want, chatfiles.IsEncryptedPDF(tt.data))
		})
	}
}

func TestApproxPDFPageCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		data      []byte
		wantCount int
		wantOK    bool
	}{
		{
			name: "PagesObjectDoesNotCount",
			data: []byte("%PDF-1.7\n1 0 obj << /Type /Pages /Count 2 >> endobj"),
		},
		{
			name:      "RepeatedPageObjects",
			data:      []byte("%PDF-1.7\n<< /Type /Page >>\n<< /Type\t/Page >>"),
			wantCount: 2,
			wantOK:    true,
		},
		{
			name: "UnknownStructure",
			data: []byte("%PDF-1.7\nxref\n0 0\ntrailer <<>>"),
		},
		{
			name: "MalformedNonPDF",
			data: []byte("plain text"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotCount, gotOK := chatfiles.ApproxPDFPageCount(tt.data)
			require.Equal(t, tt.wantCount, gotCount)
			require.Equal(t, tt.wantOK, gotOK)
		})
	}
}
