package appurl

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

// PreviewSlugPrefix starts the app slug of every preview.
const PreviewSlugPrefix = "preview--"

var previewIDFormat = regexp.MustCompile(`^[0-9a-f]{8}$`)

// PreviewID identifies a file preview that a workspace agent serves, such as
// "0123abcd".
type PreviewID string

// NewPreviewID returns the ID of the file preview of cleanedPath: the first
// 8 hex digits of its SHA-256. Publishing a path again, even after the agent
// restarts, yields the same ID. The ID is short because it is part of a
// preview hostname's first DNS label, which is limited to 63 characters.
func NewPreviewID(cleanedPath string) PreviewID {
	sum := sha256.Sum256([]byte(cleanedPath))
	return PreviewID(hex.EncodeToString(sum[:4]))
}

// UnmarshalText rejects text that is not 8 lowercase hex digits.
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
	return PreviewSlugPrefix + string(id)
}

// PreviewTarget is what a preview slug serves: files a workspace agent
// published under ID, or an HTTP server on Port. Exactly one is set.
type PreviewTarget struct {
	ID   PreviewID
	Port uint16
}

// ParsePreviewSlug returns what slug serves, such as file preview "0123abcd"
// for "preview--0123abcd" or port 8080 for "preview--8080". A port has no
// leading zeros, so each port has one hostname.
func ParsePreviewSlug(slug string) (PreviewTarget, bool) {
	target, ok := strings.CutPrefix(slug, PreviewSlugPrefix)
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

// ParsePreviewHostname returns the app and target of hostname when it is a
// preview host under the wildcard app hostname pattern: after any
// "prefix---", the wildcard part is exactly
// "preview--{ID or port}--{agent}--{workspace}--{owner}". Names cannot hold
// "--", so an app or port hostname, at most four parts, never matches.
func ParsePreviewHostname(pattern *regexp.Regexp, hostname string) (ApplicationURL, PreviewTarget, bool) {
	subdomain, ok := ExecuteHostnamePattern(pattern, strings.ToLower(hostname))
	if !ok {
		return ApplicationURL{}, PreviewTarget{}, false
	}
	app, err := ParseSubdomainAppURL(subdomain)
	if err != nil {
		return ApplicationURL{}, PreviewTarget{}, false
	}
	target, ok := ParsePreviewSlug(app.AppSlugOrPort)
	if !ok {
		return ApplicationURL{}, PreviewTarget{}, false
	}
	return app, target, true
}
