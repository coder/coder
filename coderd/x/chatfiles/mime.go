package chatfiles

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"maps"
	"mime"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"github.com/gabriel-vasile/mimetype"
	"golang.org/x/xerrors"
)

const MaxStoredFileNameBytes = 255

var (
	// ErrStoredFileNameRequired indicates that a durable file name is empty
	// after normalization.
	ErrStoredFileNameRequired = xerrors.New("stored file name is required")

	// ErrUnsupportedStoredFileType indicates that classified file bytes do not
	// map to an allowed durable file type.
	ErrUnsupportedStoredFileType = xerrors.New("unsupported attachment type")

	utf8BOM = []byte{0xEF, 0xBB, 0xBF}

	allowedStoredMediaTypes = map[string]struct{}{
		"image/png":        {},
		"image/jpeg":       {},
		"image/gif":        {},
		"image/webp":       {},
		"text/plain":       {},
		"text/markdown":    {},
		"text/csv":         {},
		"application/json": {},
		"application/pdf":  {},
	}

	recordingArtifactMediaTypes = map[string]struct{}{
		"video/mp4":  {},
		"image/jpeg": {},
	}
)

// DetectMediaType detects the base media type of the given file contents.
func DetectMediaType(data []byte) string {
	return BaseMediaType(mimetype.Detect(data).String())
}

// BaseMediaType strips parameters from a media type.
func BaseMediaType(mediaType string) string {
	if parsed, _, err := mime.ParseMediaType(mediaType); err == nil {
		return parsed
	}
	return mediaType
}

// AllowedStoredMediaTypesString returns the supported durable chat file media
// types as a comma-separated list.
func AllowedStoredMediaTypesString() string {
	return strings.Join(slices.Sorted(maps.Keys(allowedStoredMediaTypes)), ", ")
}

// IsAllowedStoredMediaType reports whether the media type is supported for
// durable chat file storage.
func IsAllowedStoredMediaType(mediaType string) bool {
	_, ok := allowedStoredMediaTypes[BaseMediaType(mediaType)]
	return ok
}

// IsInlineRenderableStoredMediaType reports whether a stored chat file may be
// served with Content-Disposition: inline. PDFs remain storable but
// download-only because browser PDF viewers have a broader active-content
// attack surface than the other media types we allow inline.
func IsInlineRenderableStoredMediaType(mediaType string) bool {
	mediaType = BaseMediaType(mediaType)
	if !IsAllowedStoredMediaType(mediaType) {
		return false
	}
	return mediaType != "application/pdf"
}

// NormalizeStoredFileName trims surrounding whitespace, strips control
// characters, and truncates the name to the durable storage byte limit
// without splitting UTF-8 runes.
func NormalizeStoredFileName(name string) string {
	name = strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, name)
	name = strings.TrimSpace(name)
	return truncateUTF8Bytes(name, MaxStoredFileNameBytes)
}

// PrepareStoredFile normalizes the display name, rejects empty normalized
// names, and classifies the file bytes using detectName when provided, so
// callers can preserve subtype detection even when the user-facing filename is
// overridden.
func PrepareStoredFile(name, detectName string, data []byte) (storedName, mediaType string, err error) {
	storedName = NormalizeStoredFileName(name)
	if storedName == "" {
		return "", "", ErrStoredFileNameRequired
	}
	if strings.TrimSpace(detectName) == "" {
		detectName = storedName
	}
	mediaType = ClassifyStoredMediaType(detectName, data)
	if !IsAllowedStoredMediaType(mediaType) {
		return "", "", xerrors.Errorf("%w %q", ErrUnsupportedStoredFileType, mediaType)
	}
	return storedName, mediaType, nil
}

// PrepareRecordingArtifact normalizes the recording artifact name, rejects
// empty normalized names, and verifies that the bytes match the expected
// recording media type.
func PrepareRecordingArtifact(name, expectedMediaType string, data []byte) (storedName, mediaType string, err error) {
	expectedMediaType = BaseMediaType(expectedMediaType)
	if _, ok := recordingArtifactMediaTypes[expectedMediaType]; !ok {
		return "", "", xerrors.Errorf("unsupported recording artifact type %q", expectedMediaType)
	}

	storedName = NormalizeStoredFileName(name)
	if storedName == "" {
		return "", "", ErrStoredFileNameRequired
	}
	mediaType = DetectMediaType(data)
	if mediaType != expectedMediaType {
		return "", "", xerrors.Errorf("recording artifact type mismatch: expected %q, detected %q", expectedMediaType, mediaType)
	}
	return storedName, mediaType, nil
}

// IsCompatibleUploadMediaType reports whether an upload request that declared
// declaredMediaType may be stored as storedMediaType after byte
// classification. Exact matches are always compatible; the compatibility
// table only covers explicit refinements like text/plain uploads that safely
// store as richer text subtypes.
func IsCompatibleUploadMediaType(declaredMediaType, storedMediaType string) bool {
	declaredMediaType = BaseMediaType(declaredMediaType)
	storedMediaType = BaseMediaType(storedMediaType)

	if declaredMediaType == storedMediaType {
		return true
	}
	if declaredMediaType != "text/plain" {
		return false
	}

	switch storedMediaType {
	case "text/markdown", "text/csv", "application/json":
		return true
	default:
		return false
	}
}

// HasSVGRootElement reports whether the provided file bytes decode to an SVG
// root element. This catches SVG content even when generic sniffers classify it
// as text or XML.
func HasSVGRootElement(data []byte) bool {
	data = bytes.TrimPrefix(data, utf8BOM)
	if len(data) == 0 {
		return false
	}

	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.Token()
		if err != nil {
			return false
		}

		switch token := token.(type) {
		case xml.ProcInst, xml.Directive, xml.Comment:
			continue
		case xml.CharData:
			if len(bytes.TrimSpace(token)) == 0 {
				continue
			}
			return false
		case xml.StartElement:
			return strings.EqualFold(token.Name.Local, "svg")
		default:
			return false
		}
	}
}

// ClassifyStoredMediaType returns the media type that durable chat storage
// would use for the given filename and bytes. Unsupported or blocked content is
// returned as its detected media type so callers can report the specific type.
func ClassifyStoredMediaType(name string, data []byte) string {
	if HasSVGRootElement(data) {
		return "image/svg+xml"
	}

	mediaType := DetectMediaType(data)
	switch mediaType {
	case "image/png", "image/jpeg", "image/gif", "image/webp",
		"text/markdown", "text/csv", "application/json",
		"application/pdf", "application/xml", "text/xml":
		return mediaType
	case "text/plain":
		return refineTextMediaType(name, data)
	default:
		if strings.HasPrefix(mediaType, "text/") {
			return "text/plain"
		}
		return mediaType
	}
}

func refineTextMediaType(name string, data []byte) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".json":
		if json.Valid(data) {
			return "application/json"
		}
	case ".md", ".markdown":
		return "text/markdown"
	case ".csv":
		return "text/csv"
	}
	return "text/plain"
}

func truncateUTF8Bytes(value string, maxBytes int) string {
	if maxBytes <= 0 || value == "" {
		return ""
	}
	if len(value) <= maxBytes {
		return value
	}

	cut := 0
	for idx := range value {
		if idx > maxBytes {
			break
		}
		cut = idx
	}
	return value[:cut]
}
