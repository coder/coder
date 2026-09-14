package config

import (
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kirsle/configdir"
	"golang.org/x/xerrors"
)

const (
	FlagName = "global-config"
)

// Root represents the configuration directory.
type Root string

// mustNotBeEmpty prevents us from accidentally writing configuration to the
// current directory. This is primarily valuable in development, where we may
// accidentally use an empty root.
func (r Root) mustNotEmpty() {
	if r == "" {
		panic("config root must not be empty")
	}
}

func (r Root) Session() File {
	r.mustNotEmpty()
	return File(filepath.Join(string(r), "session"))
}

// ReplicaID is a unique identifier for the Coder server.
func (r Root) ReplicaID() File {
	r.mustNotEmpty()
	return File(filepath.Join(string(r), "replica_id"))
}

func (r Root) URL() File {
	r.mustNotEmpty()
	return File(filepath.Join(string(r), "url"))
}

func (r Root) Organization() File {
	r.mustNotEmpty()
	return File(filepath.Join(string(r), "organization"))
}

func (r Root) DotfilesURL() File {
	r.mustNotEmpty()
	return File(filepath.Join(string(r), "dotfilesurl"))
}

func (r Root) PostgresPath() string {
	r.mustNotEmpty()
	return filepath.Join(string(r), "postgres")
}

func (r Root) PostgresPassword() File {
	r.mustNotEmpty()
	return File(filepath.Join(r.PostgresPath(), "password"))
}

func (r Root) PostgresPort() File {
	r.mustNotEmpty()
	return File(filepath.Join(r.PostgresPath(), "port"))
}

// File provides convenience methods for interacting with *os.File.
type File string

func (f File) Exists() bool {
	if f == "" {
		return false
	}
	_, err := os.Stat(string(f))
	return err == nil
}

// Delete deletes the file.
func (f File) Delete() error {
	if f == "" {
		return xerrors.Errorf("empty file path")
	}
	return os.Remove(string(f))
}

// Write writes the string to the file.
func (f File) Write(s string) error {
	if f == "" {
		return xerrors.Errorf("empty file path")
	}
	return write(string(f), 0o600, []byte(s))
}

// Read reads the file to a string. All leading and trailing whitespace
// is removed.
func (f File) Read() (string, error) {
	if f == "" {
		return "", xerrors.Errorf("empty file path")
	}
	byt, err := read(string(f))
	return strings.TrimSpace(string(byt)), err
}

// open opens a file in the configuration directory,
// creating all intermediate directories.
func open(path string, flag int, mode os.FileMode) (*os.File, error) {
	err := os.MkdirAll(filepath.Dir(path), 0o750)
	if err != nil {
		return nil, err
	}

	return os.OpenFile(path, flag, mode)
}

func write(path string, mode os.FileMode, dat []byte) error {
	fi, err := open(path, os.O_WRONLY|os.O_TRUNC|os.O_CREATE, mode)
	if err != nil {
		return err
	}
	defer fi.Close()
	_, err = fi.Write(dat)
	return err
}

func read(path string) ([]byte, error) {
	fi, err := open(path, os.O_RDONLY, 0)
	if err != nil {
		return nil, err
	}
	defer fi.Close()
	return io.ReadAll(fi)
}

func DefaultDir() string {
	configDir := configdir.LocalConfig("coderv2")
	if dir := os.Getenv("CLIDOCGEN_CONFIG_DIRECTORY"); dir != "" {
		configDir = dir
	}
	return configDir
}
