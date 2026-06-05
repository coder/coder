package chatfiles

import (
	"bytes"
	"regexp"
)

var (
	pdfHeader = []byte("%PDF-")

	pdfEncryptionMarkers = [][]byte{
		[]byte("/Encrypt"),
		[]byte("/U ("),
		[]byte("/O ("),
	}

	pdfPageObjectPattern = regexp.MustCompile(`/Type\s*/Page\b`)
)

// IsPDF reports whether data begins with the standard PDF header.
func IsPDF(data []byte) bool {
	return bytes.HasPrefix(data, pdfHeader)
}

// IsEncryptedPDF reports whether data contains common PDF encryption markers.
// It is a fast preflight heuristic, not a complete PDF parser.
func IsEncryptedPDF(data []byte) bool {
	for _, marker := range pdfEncryptionMarkers {
		if bytes.Contains(data, marker) {
			return true
		}
	}
	return false
}

// ApproxPDFPageCount estimates the number of page objects in data.
// ok is false when no page markers are found.
func ApproxPDFPageCount(data []byte) (count int, ok bool) {
	matches := pdfPageObjectPattern.FindAllIndex(data, -1)
	if len(matches) == 0 {
		return 0, false
	}
	return len(matches), true
}
