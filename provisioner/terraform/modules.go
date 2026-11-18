package terraform

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/util/xio"
	"github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/provisionersdk/tfpath"
)

const (
	terraformLockFileName = ".terraform.lock.hcl"

	// importedLockFileDir holds the lock files captured during template
	// import, one per platform. Older provisioners extract it and ignore it.
	importedLockFileDir = ".terraform/coder/lockfiles"

	// templateLockFileBackupPath holds the template's own lock file while the
	// captured one is in place.
	templateLockFileBackupPath = ".terraform/coder/template.lock.hcl"

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

type moduleWithEstimatedSize struct {
	*module
	EstimatedSize int64
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

func GetModulesArchive(root fs.FS) ([]byte, []string, error) {
	return GetModulesArchiveWithLimit(root, MaximumModuleArchiveSize)
}

// GetModulesArchiveWithLimit returns the tar archive, the skipped modules, and an error if any.
// The archive also carries the lock file init produced, so builds can reuse
// the provider versions selected at import.
func GetModulesArchiveWithLimit(root fs.FS, maxArchiveSize int64) ([]byte, []string, error) {
	lockFileContent, err := fs.ReadFile(root, terraformLockFileName)
	if err != nil && !xerrors.Is(err, fs.ErrNotExist) {
		return nil, []string{}, xerrors.Errorf("failed to read %s: %w", terraformLockFileName, err)
	}

	var m modulesFile
	modulesFileContent, err := fs.ReadFile(root, ".terraform/modules/modules.json")
	switch {
	case err == nil:
		if err := json.Unmarshal(modulesFileContent, &m); err != nil {
			return nil, []string{}, xerrors.Errorf("failed to parse modules.json: %w", err)
		}
	case !xerrors.Is(err, fs.ErrNotExist):
		return nil, []string{}, xerrors.Errorf("failed to read modules.json: %w", err)
	case len(lockFileContent) == 0:
		// No modules and no lock file: nothing to archive.
		return []byte{}, []string{}, nil
	}

	archivedModuleFiles := false
	var b bytes.Buffer

	lw := xio.NewLimitWriter(&b, maxArchiveSize)
	w := tar.NewWriter(lw)

	// Write the lock file first so modules can't crowd it out.
	if len(lockFileContent) > 0 {
		lockFilePath := importedLockFilePath(terraformPlatform())
		// Extraction doesn't create parent directories for files.
		err = w.WriteHeader(defaultDirHeader(importedLockFileDir))
		if err != nil {
			return nil, []string{}, xerrors.Errorf("failed to write %s to archive: %w", importedLockFileDir, err)
		}
		err = w.WriteHeader(defaultFileHeader(lockFilePath, len(lockFileContent)))
		if err != nil {
			return nil, []string{}, xerrors.Errorf("failed to write %s to archive: %w", terraformLockFileName, err)
		}
		if _, err := w.Write(lockFileContent); err != nil {
			return nil, []string{}, xerrors.Errorf("failed to write %s to archive: %w", terraformLockFileName, err)
		}
	}
	// Reserve room for modules.json, which is written after the modules.
	modulesFileSize := tarEntrySize(len(modulesFileContent))

	sized := make([]*moduleWithEstimatedSize, 0, len(m.Modules))
	for _, it := range m.Modules {
		sz, err := estimateModuleSize(root, it.Dir)
		if err != nil {
			return nil, []string{}, xerrors.Errorf("failed to estimate module size for %q: %w", it.Dir, err)
		}
		sized = append(sized, &moduleWithEstimatedSize{
			module:        it,
			EstimatedSize: sz,
		})
	}

	// Sort modules by estimated size descending so that we skip the largest
	slices.SortFunc(sized, func(a, b *moduleWithEstimatedSize) int {
		return int(a.EstimatedSize - b.EstimatedSize)
	})
	skippedModules := []string{}

	for _, it := range sized {
		// Check to make sure that the module is a remote module fetched by
		// Terraform. Any module that doesn't start with this path is already local,
		// and should be part of the template files already.
		if !strings.HasPrefix(it.Dir, ".terraform/modules/") {
			continue
		}

		// Flush the previous entry's padding so Remaining is accurate.
		if err := w.Flush(); err != nil {
			return nil, []string{}, xerrors.Errorf("failed to write module files archive: %w", err)
		}
		// Leave 1024 bytes for the footer
		if it.EstimatedSize > lw.Remaining()-1024-modulesFileSize {
			skippedModules = append(skippedModules, fmt.Sprintf("%s:%s", it.Key, it.Source))
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
			archivedModuleFiles = true
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
			return nil, skippedModules, err
		}
	}

	// modules.json needs the module directories as its parent on extraction.
	if archivedModuleFiles {
		err = w.WriteHeader(defaultFileHeader(".terraform/modules/modules.json", len(modulesFileContent)))
		if err != nil {
			return nil, skippedModules, xerrors.Errorf("failed to write modules.json to archive: %w", err)
		}
		if _, err := w.Write(modulesFileContent); err != nil {
			return nil, skippedModules, xerrors.Errorf("failed to write modules.json to archive: %w", err)
		}
	}

	if err := w.Close(); err != nil {
		return nil, skippedModules, xerrors.Errorf("failed to close module files archive: %w", err)
	}
	// Don't persist empty tar files in the database
	if len(lockFileContent) == 0 && !archivedModuleFiles {
		return []byte{}, skippedModules, nil
	}
	return b.Bytes(), skippedModules, nil
}

// estimateModuleSize estimates the size impact of adding the specified module
// directory to a tar archive.
func estimateModuleSize(root fs.FS, moduleDir string) (int64, error) {
	size := int64(0)
	err := fs.WalkDir(root, moduleDir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
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
			return xerrors.Errorf("file info: %w", err)
		}

		size += 512 // tar header size
		if !fileMode.IsRegular() {
			return nil // Dirs have no content size
		}

		fileSize := fileInfo.Size()
		size += fileSize
		// Pad to 512 bytes
		size += 512 - (fileSize % 512)
		return nil
	})
	if err != nil {
		return -1, err
	}
	return size, err
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

func defaultDirHeader(dirPath string) *tar.Header {
	return &tar.Header{
		Typeflag: tar.TypeDir,
		Name:     dirPath + "/",
		Mode:     0o755,
		Uid:      1000,
		Gid:      1000,
	}
}

// terraformPlatform returns the platform whose provider hashes init writes
// to the lock file.
func terraformPlatform() string {
	return runtime.GOOS + "_" + runtime.GOARCH
}

// importedLockFilePath returns the archive path of the lock file captured on
// platform.
func importedLockFilePath(platform string) string {
	return path.Join(importedLockFileDir, platform+".lock.hcl")
}

// restoreImportedLockFile replaces the template's lock file with the one
// captured during import, and reports whether it did. Only a lock file that
// moduleArchive delivered counts: the template's source archive is extracted
// into the same directory first and could hold a file at that path. Lock
// files captured on another platform are ignored, because they may lack this
// platform's hashes.
func restoreImportedLockFile(files tfpath.Layout, moduleArchive []byte) (bool, error) {
	name := importedLockFilePath(terraformPlatform())
	delivered, err := archiveHasFile(moduleArchive, name)
	if err != nil {
		return false, xerrors.Errorf("read module archive: %w", err)
	}
	if !delivered {
		return false, nil
	}
	src := filepath.Join(files.WorkDirectory(), filepath.FromSlash(name))
	if _, err := os.Stat(src); err != nil {
		if xerrors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, xerrors.Errorf("stat imported lock file: %w", err)
	}

	backup := filepath.Join(files.WorkDirectory(), filepath.FromSlash(templateLockFileBackupPath))
	err = os.Rename(files.TerraformLockFile(), backup)
	if err != nil && !xerrors.Is(err, fs.ErrNotExist) {
		return false, xerrors.Errorf("set aside template lock file: %w", err)
	}
	err = os.Rename(src, files.TerraformLockFile())
	if err != nil {
		err = xerrors.Errorf("move imported lock file into place: %w", err)
		return false, errors.Join(err, restoreTemplateLockFile(files))
	}
	return true, nil
}

// archiveHasFile reports whether the tar archive holds a regular file named
// name.
func archiveHasFile(archive []byte, name string) (bool, error) {
	r := tar.NewReader(bytes.NewReader(archive))
	for {
		header, err := r.Next()
		if errors.Is(err, io.EOF) {
			return false, nil
		}
		if err != nil {
			return false, err
		}
		if header.Typeflag == tar.TypeReg && path.Clean(header.Name) == name {
			return true, nil
		}
	}
}

// restoreTemplateLockFile undoes restoreImportedLockFile.
func restoreTemplateLockFile(files tfpath.Layout) error {
	dst := files.TerraformLockFile()
	err := os.Rename(filepath.Join(files.WorkDirectory(), filepath.FromSlash(templateLockFileBackupPath)), dst)
	if err == nil {
		return nil
	}
	if !xerrors.Is(err, fs.ErrNotExist) {
		return xerrors.Errorf("restore template lock file: %w", err)
	}
	err = os.Remove(dst)
	if err != nil && !xerrors.Is(err, fs.ErrNotExist) {
		return xerrors.Errorf("remove imported lock file: %w", err)
	}
	return nil
}

// tarEntrySize returns the archive size of a file of the given length.
func tarEntrySize(length int) int64 {
	return 512 + (int64(length)+511)/512*512
}
