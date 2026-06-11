package chatfiles

import (
	"bytes"
	"regexp"
)

var (
	pdfHeader = []byte("%PDF-")

	// pdfEncryptDictMarker points at the trailer's encryption dictionary and
	// is present in every encrypted PDF. The /U and /O entries only live inside
	// that dictionary, so scanning for them adds no recall while risking false
	// positives against incidental content, which would hard-fail a valid PDF.
	pdfEncryptDictMarker = []byte("/Encrypt")

	pdfPageObjectPattern = regexp.MustCompile(`/Type\s*/Page\b`)
)

// IsPDF reports whether data begins with the standard PDF header.
func IsPDF(data []byte) bool {
	return bytes.HasPrefix(data, pdfHeader)
}

// IsEncryptedPDF reports whether data contains common PDF encryption markers.
// It is a fast preflight heuristic, not a complete PDF parser.
func IsEncryptedPDF(data []byte) bool {
	return bytes.Contains(data, pdfEncryptDictMarker)
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
