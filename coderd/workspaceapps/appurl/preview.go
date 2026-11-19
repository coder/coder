package appurl

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"

	"golang.org/x/xerrors"
)

var previewIDFormat = regexp.MustCompile(`^[0-9a-f]{8}$`)

// PreviewID identifies a file preview that a workspace agent serves, such as
// "0123abcd".
type PreviewID string

// NewPreviewID returns the ID of the file preview of cleanedPath: the first
// 8 hex digits of its SHA-256. Publishing a path again, even after the agent
// restarts, yields the same ID.
func NewPreviewID(cleanedPath string) PreviewID {
	sum := sha256.Sum256([]byte(cleanedPath))
	return PreviewID(hex.EncodeToString(sum[:4]))
}

// UnmarshalText accepts only the format of a preview ID.
func (id *PreviewID) UnmarshalText(text []byte) error {
	if !previewIDFormat.Match(text) {
		return xerrors.Errorf("invalid preview ID %q", text)
	}
	*id = PreviewID(text)
	return nil
}
