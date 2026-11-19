package appurl

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

const previewSlugPrefix = "preview--"

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

// Slug returns the app slug that serves the preview, such as
// "preview--0123abcd".
func (id PreviewID) Slug() string {
	return previewSlugPrefix + string(id)
}

// PreviewTarget is what a preview slug serves: files a workspace agent
// published under ID, or an HTTP server on Port. Exactly one is set.
type PreviewTarget struct {
	ID   PreviewID
	Port uint16
}

// Slug returns the app slug that serves t, such as "preview--0123abcd" or
// "preview--8080".
func (t PreviewTarget) Slug() string {
	if t.Port != 0 {
		return previewSlugPrefix + strconv.FormatUint(uint64(t.Port), 10)
	}
	return t.ID.Slug()
}

// ParsePreviewSlug returns what slug serves, such as file preview "0123abcd"
// for "preview--0123abcd" or port 8080 for "preview--8080". A port has no
// leading zeros, so each port has one hostname.
func ParsePreviewSlug(slug string) (PreviewTarget, bool) {
	target, ok := strings.CutPrefix(slug, previewSlugPrefix)
	if !ok {
		return PreviewTarget{}, false
	}
	if previewIDFormat.MatchString(target) {
		return PreviewTarget{ID: PreviewID(target)}, true
	}
	port, err := strconv.ParseUint(target, 10, 16)
	if err != nil || port == 0 || strconv.FormatUint(port, 10) != target {
		return PreviewTarget{}, false
	}
	return PreviewTarget{Port: uint16(port)}, true
}

// HasPreviewSlugPrefix reports whether s starts like a preview slug, whether
// or not the rest parses.
func HasPreviewSlugPrefix(s string) bool {
	return strings.HasPrefix(s, previewSlugPrefix)
}

// IsPreviewHostname reports whether hostname is a preview host under any
// wildcard app hostname, including those of workspace proxies, whose patterns
// can add a suffix to the first label.
func IsPreviewHostname(hostname string) bool {
	label, _, _ := strings.Cut(strings.ToLower(hostname), ".")
	// Drop any "prefix---" so a prefixed preview host still counts.
	if i := strings.LastIndex(label, "---"+previewSlugPrefix); i >= 0 {
		label = label[i+len("---"):]
	}

	parts := strings.Split(label, "--")
	if len(parts) < 5 {
		return false
	}
	_, ok := ParsePreviewSlug(parts[0] + "--" + parts[1])
	return ok
}
