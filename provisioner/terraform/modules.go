package terraform

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/xio"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
)

const (
	// MaximumModuleArchiveSize limits the total size of a module archive.
	// At some point, the user should take steps to reduce the size of their
	// template modules, as this can lead to performance issues
	// TODO: Determine what a reasonable limit is for modules
	//  If we start hitting this limit, we might want to consider adding
	//  configurable filters? Files like images could blow up the size of a
	//  module.
	MaximumModuleArchiveSize = 20 * 1024 * 1024 // 20MB
)

type module struct {
	Source  string `json:"Source"`
	Version string `json:"Version"`
	Key     string `json:"Key"`
	Dir     string `json:"Dir"`
}

type modulesFile struct {
	Modules []*module `json:"Modules"`
}

func parseModulesFile(filePath string) ([]*proto.Module, error) {
	modules := &modulesFile{}
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, xerrors.Errorf("read modules file: %w", err)
	}
	if err := json.Unmarshal(data, modules); err != nil {
		return nil, xerrors.Errorf("unmarshal modules file: %w", err)
	}
	protoModules := make([]*proto.Module, len(modules.Modules))
	for i, m := range modules.Modules {
		protoModules[i] = &proto.Module{Source: m.Source, Version: m.Version, Key: m.Key}
	}
	return protoModules, nil
}

// getModules returns the modules from the modules file if it exists.
// It returns nil if the file does not exist.
// Modules become available after terraform init.
func getModules(files tfpath.Layout) ([]*proto.Module, error) {
	filePath := files.ModulesFilePath()
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, nil
	}
	modules, err := parseModulesFile(filePath)
	if err != nil {
		return nil, xerrors.Errorf("parse modules file: %w", err)
	}
	filteredModules := []*proto.Module{}
	for _, m := range modules {
		// Empty string means root module. It's always present, so we skip it.
		if m.Source == "" {
			continue
		}
		filteredModules = append(filteredModules, m)
	}
	return filteredModules, nil
}

func GetModulesArchive(root fs.FS) ([]byte, error) {
	modulesFileContent, err := fs.ReadFile(root, ".terraform/modules/modules.json")
	if err != nil {
		if xerrors.Is(err, fs.ErrNotExist) {
			return []byte{}, nil
		}
		return nil, xerrors.Errorf("failed to read modules.json: %w", err)
	}
	var m modulesFile
	if err := json.Unmarshal(modulesFileContent, &m); err != nil {
		return nil, xerrors.Errorf("failed to parse modules.json: %w", err)
	}

	empty := true
	var b bytes.Buffer

	lw := xio.NewLimitWriter(&b, MaximumModuleArchiveSize)
	w := tar.NewWriter(lw)

	for _, it := range m.Modules {
		// Check to make sure that the module is a remote module fetched by
		// Terraform. Any module that doesn't start with this path is already local,
		// and should be part of the template files already.
		if !strings.HasPrefix(it.Dir, ".terraform/modules/") {
			continue
		}

		err := fs.WalkDir(root, it.Dir, func(filePath string, d fs.DirEntry, err error) error {
			if err != nil {
				return xerrors.Errorf("failed to create modules archive: %w", err)
			}
			fileMode := d.Type()
			if !fileMode.IsRegular() && !fileMode.IsDir() {
				return nil
			}

			// .git directories are not needed in the archive and only cause
			// hash differences for identical modules.
			if fileMode.IsDir() && d.Name() == ".git" {
				return fs.SkipDir
			}

			fileInfo, err := d.Info()
			if err != nil {
				return xerrors.Errorf("failed to archive module file %q: %w", filePath, err)
			}
			header, err := fileHeader(filePath, fileMode, fileInfo)
			if err != nil {
				return xerrors.Errorf("failed to archive module file %q: %w", filePath, err)
			}
			err = w.WriteHeader(header)
			if err != nil {
				return xerrors.Errorf("failed to add module file %q to archive: %w", filePath, err)
			}

			if !fileMode.IsRegular() {
				return nil
			}
			empty = false
			file, err := root.Open(filePath)
			if err != nil {
				return xerrors.Errorf("failed to open module file %q while archiving: %w", filePath, err)
			}
			defer file.Close()
			_, err = io.Copy(w, file)
			if err != nil {
				return xerrors.Errorf("failed to copy module file %q while archiving: %w", filePath, err)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	err = w.WriteHeader(defaultFileHeader(".terraform/modules/modules.json", len(modulesFileContent)))
	if err != nil {
		return nil, xerrors.Errorf("failed to write modules.json to archive: %w", err)
	}
	if _, err := w.Write(modulesFileContent); err != nil {
		return nil, xerrors.Errorf("failed to write modules.json to archive: %w", err)
	}

	if err := w.Close(); err != nil {
		return nil, xerrors.Errorf("failed to close module files archive: %w", err)
	}
	// Don't persist empty tar files in the database
	if empty {
		return []byte{}, nil
	}
	return b.Bytes(), nil
}

func fileHeader(filePath string, fileMode fs.FileMode, fileInfo fs.FileInfo) (*tar.Header, error) {
	header, err := tar.FileInfoHeader(fileInfo, "")
	if err != nil {
		return nil, xerrors.Errorf("failed to archive module file %q: %w", filePath, err)
	}
	header.Name = filePath
	if fileMode.IsDir() {
		header.Name += "/"
	}
	// Erase a bunch of metadata that we don't need so that we get more consistent
	// hashes from the resulting archive.
	header.AccessTime = time.Time{}
	header.ChangeTime = time.Time{}
	header.ModTime = time.Time{}
	header.Uid = 1000
	header.Uname = ""
	header.Gid = 1000
	header.Gname = ""

	return header, nil
}

func defaultFileHeader(filePath string, length int) *tar.Header {
	return &tar.Header{
		Name: filePath,
		Size: int64(length),
		Mode: 0o644,
		Uid:  1000,
		Gid:  1000,
	}
}
