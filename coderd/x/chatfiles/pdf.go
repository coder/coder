package chatfiles

import (
	"bytes"
	"regexp"
)

var (
	pdfHeader = []byte("%PDF-")

	// pdfEncryptDictPattern matches an /Encrypt dictionary entry whose value
	// is an indirect reference ("12 0 R") or an inline dictionary ("<<").
	// Every encrypted PDF carries this entry in its trailer or XRef-stream
	// dictionary, which is never compressed, so matching the key-value shape
	// keeps full recall while ignoring incidental "/Encrypt" prose in document
	// text or metadata, which must not hard-fail a valid PDF.
	pdfEncryptDictPattern = regexp.MustCompile(`/Encrypt(\s+\d+\s+\d+\s+R\b|\s*<<)`)

	pdfPageObjectPattern = regexp.MustCompile(`/Type\s*/Page\b`)
)

// IsPDF reports whether data begins with the standard PDF header.
func IsPDF(data []byte) bool {
	return bytes.HasPrefix(data, pdfHeader)
}

// IsEncryptedPDF reports whether data carries a trailer /Encrypt dictionary
// entry. It is a fast preflight heuristic, not a complete PDF parser.
func IsEncryptedPDF(data []byte) bool {
	return pdfEncryptDictPattern.Match(data)
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
