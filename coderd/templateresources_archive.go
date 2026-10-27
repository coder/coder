package coderd

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"path"
	"slices"
	"strings"

	"golang.org/x/xerrors"
)

// Match the provisioner's per-file extraction limit.
const templateResourceFileMaxBytes = 10 << 20

// normalizeTemplateResourceArchive snapshots the complete root without trusting
// archive paths, metadata, or declared sizes. Auxiliary files and local modules
// are retained, but backend configuration is forbidden in every Terraform file.
func normalizeTemplateResourceArchive(data []byte, maxBytes int) ([]byte, string, error) {
	invalid := func(message string) ([]byte, string, error) {
		return nil, "", &templateResourceHTTPError{http.StatusBadRequest, "Invalid shared resource archive: " + message}
	}
	if len(data) > maxBytes {
		return invalid("archive exceeds maximum size")
	}
	type archiveEntry struct {
		name string
		mode int64
		dir  bool
		data []byte
	}
	var entries []archiveEntry
	seen := make(map[string]bool)
	input := bytes.NewReader(data)
	reader := tar.NewReader(input)
	var total int64
	var source string
	rootTerraform := false
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return invalid(err.Error())
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeDir {
			return invalid(fmt.Sprintf("unsupported entry type for %q", header.Name))
		}
		// The upload endpoint's ZIP conversion preserves Go file type bits
		// in Mode. Do not turn a converted symlink or device into a file.
		if header.Mode < 0 || header.Mode&int64(fs.ModeType)&^int64(fs.ModeDir) != 0 {
			return invalid(fmt.Sprintf("unsupported entry mode for %q", header.Name))
		}
		for key := range header.PAXRecords {
			if strings.HasPrefix(key, "GNU.sparse.") {
				return invalid("sparse files are not supported")
			}
		}
		if len(seen) >= 10000 || len(header.Name) > 4096 {
			return invalid("too many entries or path exceeds 4096 bytes")
		}
		if header.Name == "" || path.IsAbs(header.Name) || strings.ContainsAny(header.Name, "\\:") {
			return invalid(fmt.Sprintf("unsafe path %q", header.Name))
		}
		for component := range strings.SplitSeq(header.Name, "/") {
			if component == ".." {
				return invalid(fmt.Sprintf("unsafe path %q", header.Name))
			}
		}
		name := path.Clean(header.Name)
		// Uploaded runtime files must not seed state or select a backend or
		// workspace when Coder has no prior state. Match the root component
		// to reject empty directories and children without directory entries.
		rootName, _, _ := strings.Cut(name, "/")
		switch rootName {
		case "terraform.tfstate", "terraform.tfstate.backup", ".terraform", "terraform.tfstate.d":
			return invalid(fmt.Sprintf("reserved Terraform runtime path %q", name))
		}
		if _, exists := seen[name]; exists {
			return invalid(fmt.Sprintf("duplicate path %q", name))
		}
		dir := header.Typeflag == tar.TypeDir || header.Mode&int64(fs.ModeDir) != 0
		seen[name] = dir
		if name == "." && !dir {
			return invalid("root entry must be a directory")
		}
		if header.Size < 0 || header.Size > templateResourceFileMaxBytes || (dir && header.Size != 0) {
			return invalid(fmt.Sprintf("invalid or excessive size for %q", name))
		}
		total += header.Size
		if total > int64(maxBytes) {
			return invalid("expanded archive exceeds maximum size")
		}
		content, err := io.ReadAll(io.LimitReader(reader, header.Size+1))
		if err != nil {
			return invalid(err.Error())
		}
		if int64(len(content)) != header.Size {
			return invalid(fmt.Sprintf("incomplete file %q", name))
		}
		if !dir && (strings.HasSuffix(name, ".tf") || strings.HasSuffix(name, ".tf.json")) {
			if err := validateTemplateResourceFile(content, name); err != nil {
				return nil, "", err
			}
			if path.Dir(name) == "." && !strings.HasPrefix(name, ".") && !strings.HasPrefix(name, "~") {
				rootTerraform = true
			}
		}
		if name == "main.tf" && !dir {
			source = string(content)
		}
		if name != "." {
			entries = append(entries, archiveEntry{name: name, mode: header.Mode, dir: dir, data: content})
		}
	}
	// Do not silently ignore concatenated archives or data after the TAR footer.
	for _, b := range data[len(data)-input.Len():] {
		if b != 0 {
			return invalid("unexpected data after archive footer")
		}
	}
	if !rootTerraform {
		return invalid("at least one root .tf or .tf.json file is required")
	}
	for _, entry := range entries {
		for parent := path.Dir(entry.name); parent != "."; parent = path.Dir(parent) {
			if dir, exists := seen[parent]; exists {
				if !dir {
					return invalid(fmt.Sprintf("file %q is also a parent directory", parent))
				}
			} else {
				if len(seen) >= 10000 {
					return invalid("too many entries")
				}
				// The provisioner expects parents before their files, including
				// when the uploaded TAR omitted explicit directory entries.
				seen[parent] = true
				entries = append(entries, archiveEntry{name: parent, dir: true})
			}
		}
	}
	slices.SortFunc(entries, func(a, b archiveEntry) int { return strings.Compare(a.name, b.name) })
	var output bytes.Buffer
	writer := tar.NewWriter(&templateResourceArchiveWriter{writer: &output, remaining: maxBytes})
	for _, entry := range entries {
		header := &tar.Header{Name: entry.name, Typeflag: tar.TypeReg, Mode: 0o600 | (entry.mode & 0o111), Size: int64(len(entry.data))}
		if entry.dir {
			header.Typeflag, header.Mode = tar.TypeDir, 0o700
		}
		if err := writer.WriteHeader(header); err != nil {
			return invalid(err.Error())
		}
		if _, err := writer.Write(entry.data); err != nil {
			return invalid(err.Error())
		}
	}
	if err := writer.Close(); err != nil {
		return invalid(err.Error())
	}
	return output.Bytes(), source, nil
}

type templateResourceArchiveWriter struct {
	writer    io.Writer
	remaining int
}

func (w *templateResourceArchiveWriter) Write(p []byte) (int, error) {
	if len(p) > w.remaining {
		return 0, xerrors.New("normalized archive exceeds maximum size")
	}
	n, err := w.writer.Write(p)
	w.remaining -= n
	return n, err
}
