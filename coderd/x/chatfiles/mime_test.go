package chatfiles_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/coder/coder/v2/coderd/x/chatfiles"
)

func TestDetectMediaType_WebP(t *testing.T) {
	t.Parallel()

	data := append([]byte("RIFF"), []byte{0x24, 0x00, 0x00, 0x00}...)
	data = append(data, []byte("WEBPVP8 ")...)
	require.Equal(t, "image/webp", chatfiles.DetectMediaType(data))
}

func TestClassifyStoredMediaType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		fileName string
		data     []byte
		want     string
	}{
		{
			name:     "PlainText",
			fileName: "build.log",
			data:     []byte("build succeeded\n"),
			want:     "text/plain",
		},
		{
			name:     "MarkdownFromExtension",
			fileName: "notes.md",
			data:     []byte("# Release notes\n"),
			want:     "text/markdown",
		},
		{
			name:     "CSVFromDetector",
			fileName: "report.txt",
			data:     []byte("name,count\nwidgets,3\n"),
			want:     "text/csv",
		},
		{
			name:     "JSONFromDetector",
			fileName: "payload.txt",
			data:     []byte(`{"ok":true}`),
			want:     "application/json",
		},
		{
			name:     "UppercaseJSONExtension",
			fileName: "data.JSON",
			data:     []byte(`{"ok":true}`),
			want:     "application/json",
		},
		{
			name:     "InvalidJSONExtensionFallsBackToPlainText",
			fileName: "broken.json",
			data:     []byte("not json"),
			want:     "text/plain",
		},
		{
			name:     "UppercaseMDExtension",
			fileName: "NOTES.MD",
			data:     []byte("# Notes\n"),
			want:     "text/markdown",
		},
		{
			name:     "PDF",
			fileName: "report.pdf",
			data:     []byte("%PDF-1.7\n"),
			want:     "application/pdf",
		},
		{
			name:     "BinaryOctetStream",
			fileName: "data.bin",
			data:     []byte{0x00, 0x01, 0x02, 0x03, 0x04, 0x05},
			want:     "application/octet-stream",
		},
		{
			name:     "HTMLFallsBackToTextPlain",
			fileName: "snippet.txt",
			data:     []byte("<!DOCTYPE html><html><body>hello</body></html>"),
			want:     "text/plain",
		},
		{
			name:     "XMLStaysBlocked",
			fileName: "note.xml",
			data:     []byte(`<?xml version="1.0"?><note><to>Tove</to></note>`),
			want:     "text/xml",
		},
		{
			name:     "SVGBlockedEvenWhenNamedText",
			fileName: "notes.txt",
			data:     []byte(`<svg xmlns="http://www.w3.org/2000/svg"><text>Hello</text></svg>`),
			want:     "image/svg+xml",
		},
		{
			name:     "MarkdownMentioningSVGStaysMarkdown",
			fileName: "notes.md",
			data:     []byte("# SVG Example\n<svg width=\"100\">...</svg>"),
			want:     "text/markdown",
		},
		{
			name:     "CSVMentioningSVGStaysCSV",
			fileName: "report.csv",
			data:     []byte("name,icon\nlogo,<svg><rect/></svg>\n"),
			want:     "text/csv",
		},
		{
			name:     "TextMentioningSVGStaysPlainText",
			fileName: "main.go",
			data:     []byte("package main\n// renders <svg> tags\n"),
			want:     "text/plain",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatfiles.ClassifyStoredMediaType(tt.fileName, tt.data))
		})
	}
}

func TestPrepareStoredFile(t *testing.T) {
	t.Parallel()

	t.Run("UsesDetectNameForSubtypeRefinement", func(t *testing.T) {
		t.Parallel()

		name, mediaType, err := chatfiles.PrepareStoredFile(
			"payload.txt",
			"report.json",
			[]byte(`{"ok":true}`),
		)
		require.NoError(t, err)
		require.Equal(t, "payload.txt", name)
		require.Equal(t, "application/json", mediaType)
	})

	t.Run("StripsControlCharactersAndTrimsExposedWhitespace", func(t *testing.T) {
		t.Parallel()

		name, mediaType, err := chatfiles.PrepareStoredFile(
			"\x00 release\t notes.txt \x00",
			"release-notes.txt",
			[]byte("hello"),
		)
		require.NoError(t, err)
		require.Equal(t, "release notes.txt", name)
		require.Equal(t, "text/plain", mediaType)
	})

	t.Run("RejectsEmptyNormalizedName", func(t *testing.T) {
		t.Parallel()

		_, _, err := chatfiles.PrepareStoredFile(
			" \r\n\t ",
			"notes.txt",
			[]byte("hello"),
		)
		require.ErrorIs(t, err, chatfiles.ErrStoredFileNameRequired)
	})

	t.Run("RejectsUnsupportedStoredFileType", func(t *testing.T) {
		t.Parallel()

		_, _, err := chatfiles.PrepareStoredFile(
			"evil.svg",
			"evil.svg",
			[]byte(`<svg xmlns="http://www.w3.org/2000/svg"><rect/></svg>`),
		)
		require.ErrorIs(t, err, chatfiles.ErrUnsupportedStoredFileType)
		require.ErrorContains(t, err, "image/svg+xml")
	})

	t.Run("TruncatesNamesAtRuneBoundaries", func(t *testing.T) {
		t.Parallel()

		name, _, err := chatfiles.PrepareStoredFile(
			strings.Repeat("界", 100),
			"notes.txt",
			[]byte("hello"),
		)
		require.NoError(t, err)
		require.Equal(t, strings.Repeat("界", 85), name)
		require.Equal(t, 255, len(name))
	})
}

func TestPrepareRecordingArtifact(t *testing.T) {
	t.Parallel()

	t.Run("MP4", func(t *testing.T) {
		t.Parallel()

		name, mediaType, err := chatfiles.PrepareRecordingArtifact(
			"recording.mp4",
			"video/mp4",
			[]byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0x00, 0x00, 0x00, 0x00, 'm', 'p', '4', '1', 'i', 's', 'o', 'm'},
		)
		require.NoError(t, err)
		require.Equal(t, "recording.mp4", name)
		require.Equal(t, "video/mp4", mediaType)
	})

	t.Run("JPEG", func(t *testing.T) {
		t.Parallel()

		name, mediaType, err := chatfiles.PrepareRecordingArtifact(
			"thumbnail.jpg",
			"image/jpeg",
			[]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00},
		)
		require.NoError(t, err)
		require.Equal(t, "thumbnail.jpg", name)
		require.Equal(t, "image/jpeg", mediaType)
	})

	t.Run("TypeMismatch", func(t *testing.T) {
		t.Parallel()

		_, _, err := chatfiles.PrepareRecordingArtifact(
			"recording.mp4",
			"video/mp4",
			[]byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00},
		)
		require.ErrorContains(t, err, "recording artifact type mismatch")
	})

	t.Run("RejectsEmptyNormalizedName", func(t *testing.T) {
		t.Parallel()

		_, _, err := chatfiles.PrepareRecordingArtifact(
			" \r\n\t ",
			"video/mp4",
			[]byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'm', 'p', '4', '2', 0x00, 0x00, 0x00, 0x00, 'm', 'p', '4', '1', 'i', 's', 'o', 'm'},
		)
		require.ErrorIs(t, err, chatfiles.ErrStoredFileNameRequired)
	})

	t.Run("UnsupportedExpectedType", func(t *testing.T) {
		t.Parallel()

		_, _, err := chatfiles.PrepareRecordingArtifact(
			"recording.webm",
			"video/webm",
			[]byte("webm"),
		)
		require.ErrorContains(t, err, "unsupported recording artifact type")
	})
}

func TestIsCompatibleUploadMediaType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		declared string
		stored   string
		want     bool
	}{
		{
			name:     "ExactMatch",
			declared: "text/plain",
			stored:   "text/plain",
			want:     true,
		},
		{
			name:     "OctetStreamMatchesPNG",
			declared: "application/octet-stream",
			stored:   "image/png",
			want:     true,
		},
		{
			name:     "OctetStreamMatchesJSON",
			declared: "application/octet-stream",
			stored:   "application/json",
			want:     true,
		},
		{
			name:     "TextPlainRefinesToMarkdown",
			declared: "text/plain",
			stored:   "text/markdown",
			want:     true,
		},
		{
			name:     "TextPlainRefinesToCSV",
			declared: "text/plain",
			stored:   "text/csv",
			want:     true,
		},
		{
			name:     "TextPlainRefinesToJSON",
			declared: "text/plain",
			stored:   "application/json",
			want:     true,
		},
		{
			name:     "TextPlainDoesNotRefineToPNG",
			declared: "text/plain",
			stored:   "image/png",
			want:     false,
		},
		{
			name:     "JSONDoesNotRefineToPlainText",
			declared: "application/json",
			stored:   "text/plain",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, chatfiles.IsCompatibleUploadMediaType(tt.declared, tt.stored))
		})
	}
}

func TestIsAllowedStoredMediaType(t *testing.T) {
	t.Parallel()

	require.True(t, chatfiles.IsAllowedStoredMediaType("text/plain; charset=utf-8"))
	require.True(t, chatfiles.IsAllowedStoredMediaType("text/markdown"))
	require.True(t, chatfiles.IsAllowedStoredMediaType("text/csv"))
	require.True(t, chatfiles.IsAllowedStoredMediaType("application/json"))
	require.True(t, chatfiles.IsAllowedStoredMediaType("application/pdf"))
	require.True(t, chatfiles.IsAllowedStoredMediaType("image/png"))
	require.False(t, chatfiles.IsAllowedStoredMediaType("image/svg+xml"))
	require.False(t, chatfiles.IsAllowedStoredMediaType("image/avif"))
	require.False(t, chatfiles.IsAllowedStoredMediaType("application/zip"))
}

func TestIsInlineRenderableStoredMediaType(t *testing.T) {
	t.Parallel()

	require.True(t, chatfiles.IsInlineRenderableStoredMediaType("text/plain; charset=utf-8"))
	require.True(t, chatfiles.IsInlineRenderableStoredMediaType("text/markdown"))
	require.True(t, chatfiles.IsInlineRenderableStoredMediaType("image/png"))
	require.False(t, chatfiles.IsInlineRenderableStoredMediaType("application/pdf"))
	require.False(t, chatfiles.IsInlineRenderableStoredMediaType("image/svg+xml"))
}

func TestHasSVGRootElement(t *testing.T) {
	t.Parallel()

	require.True(t, chatfiles.HasSVGRootElement([]byte(`<?xml version="1.0"?><svg xmlns="http://www.w3.org/2000/svg"></svg>`)))
	require.True(t, chatfiles.HasSVGRootElement([]byte("\xef\xbb\xbf<svg></svg>")))
	require.False(t, chatfiles.HasSVGRootElement([]byte("<html><body>not svg</body></html>")))
	require.False(t, chatfiles.HasSVGRootElement([]byte("# SVG Example\n<svg width=\"100\">...</svg>")))
	require.False(t, chatfiles.HasSVGRootElement([]byte("name,icon\nlogo,<svg><rect/></svg>\n")))
}
