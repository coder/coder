// Package workspaceartifacts validates and preserves declared execution results.
package workspaceartifacts

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

// MaxBundleBytes matches the bounded agent bundle transport.
const MaxBundleBytes = 110 * 1024 * 1024

// File is a complete, verified file from a workspace bundle.
type File struct {
	Path     string
	Name     string
	MIMEType string
	SHA256   string
	Data     []byte
}

// ValidateBundle rejects partial collections before any results are persisted.
// Every declared path must have at least one complete file. Glob declarations
// may produce several files, but overlapping declarations cannot alias a result.
func ValidateBundle(bundle []byte, requested []string) ([]File, error) {
	if len(bundle) > MaxBundleBytes || len(bundle) < 1024 || len(bundle)%512 != 0 ||
		!bytes.Equal(bundle[len(bundle)-1024:], make([]byte, 1024)) {
		return nil, xerrors.New("incomplete or oversized result archive")
	}
	required := make(map[string]bool, len(requested))
	for _, p := range requested {
		if p == "" {
			return nil, xerrors.New("empty result path")
		}
		if _, exists := required[p]; exists {
			return nil, xerrors.New("duplicate result declaration")
		}
		required[p] = false
	}
	reader := bytes.NewReader(bundle)
	tr := tar.NewReader(reader)
	entries := make(map[string][]byte)
	var manifest *workspacesdk.BundleFilesManifest
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, xerrors.Errorf("read result archive: %w", err)
		}
		if manifest != nil || header.Typeflag != tar.TypeReg || !fs.ValidPath(header.Name) ||
			strings.Contains(header.Name, "\\") || header.Size < 0 || header.Size > MaxBundleBytes {
			return nil, xerrors.New("invalid result archive entry")
		}
		if _, exists := entries[header.Name]; exists {
			return nil, xerrors.New("duplicate result archive entry")
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return nil, xerrors.Errorf("read result bytes: %w", err)
		}
		if header.Name == "manifest.json" {
			manifest = new(workspacesdk.BundleFilesManifest)
			if err := json.Unmarshal(data, manifest); err != nil {
				return nil, xerrors.Errorf("read result manifest: %w", err)
			}
		} else {
			if !strings.HasPrefix(header.Name, "files/") || len(entries) >= 10000 {
				return nil, xerrors.New("unexpected result archive entry")
			}
			entries[header.Name] = data
		}
	}
	if len(bytes.Trim(bundle[len(bundle)-reader.Len():], "\x00")) != 0 {
		return nil, xerrors.New("unexpected data after result archive")
	}
	if manifest == nil || manifest.Truncated || len(manifest.Errors) != 0 ||
		!slices.Equal(manifest.Requested, requested) || len(manifest.Files) != len(entries) {
		return nil, xerrors.New("result preservation is incomplete")
	}
	files := make([]File, 0, len(entries))
	seenPaths := make(map[string]bool, len(entries))
	for _, entry := range manifest.Files {
		data, exists := entries[entry.ArchivePath]
		source := strings.ReplaceAll(entry.Path, "\\", "/")
		archivePath := strings.TrimPrefix(source, "/")
		if len(archivePath) >= 2 && archivePath[1] == ':' {
			archivePath = archivePath[:1] + archivePath[2:]
		}
		_, declared := required[entry.Requested]
		absolute := strings.HasPrefix(source, "/") || (len(source) > 2 && source[1] == ':' && source[2] == '/')
		if !exists || !declared || seenPaths[source] || source == "" || path.Clean(source) != source || !fs.ValidPath(archivePath) ||
			!absolute ||
			entry.ArchivePath != "files/"+archivePath || entry.Truncated || entry.Size < 0 ||
			entry.Size != entry.BytesWritten || entry.Size != int64(len(data)) {
			return nil, xerrors.New("result manifest does not match complete file bytes")
		}
		delete(entries, entry.ArchivePath)
		seenPaths[source] = true
		required[entry.Requested] = true
		sum := sha256.Sum256(data)
		files = append(files, File{
			Path: entry.Path, Name: path.Base(source), MIMEType: http.DetectContentType(data),
			SHA256: hex.EncodeToString(sum[:]), Data: data,
		})
	}
	for _, found := range required {
		if !found {
			return nil, xerrors.New("declared result path is missing")
		}
	}
	return files, nil
}
