package terraform

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/provisionersdk/proto"
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

func getModulesFilePath(workdir string) string {
	return filepath.Join(workdir, ".terraform", "modules", "modules.json")
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
func getModules(workdir string) ([]*proto.Module, error) {
	filePath := getModulesFilePath(workdir)
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

func getModulesArchive(root fs.FS) ([]byte, error) {
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
	w := tar.NewWriter(&b)

	for _, it := range m.Modules {
		// Check to make sure that the module is a remote module fetched by
		// Terraform. Any module that doesn't start with this path is already local,
		// and should be part of the template files already.
		if !strings.HasPrefix(it.Dir, ".terraform/modules/") {
			continue
		}

		err := fs.WalkDir(root, it.Dir, func(filePath string, info fs.DirEntry, err error) error {
			if err != nil {
				return xerrors.Errorf("failed to create modules archive: %w", err)
			}
			if info.IsDir() {
				return nil
			}

			content, err := fs.ReadFile(root, filePath)
			if err != nil {
				return xerrors.Errorf("failed to read module file while archiving: %w", err)
			}
			empty = false
			err = w.WriteHeader(&tar.Header{
				Name: filePath,
				Size: int64(len(content)),
				Mode: 0o644,
				Uid:  1000,
				Gid:  1000,
			})
			if err != nil {
				return xerrors.Errorf("failed to add module file to archive: %w", err)
			}
			if _, err = w.Write(content); err != nil {
				return xerrors.Errorf("failed to write module file to archive: %w", err)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	err = w.WriteHeader(&tar.Header{
		Name: ".terraform/modules/modules.json",
		Size: int64(len(modulesFileContent)),
		Mode: 0o644,
		Uid:  1000,
		Gid:  1000,
	})
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
