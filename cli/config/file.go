package config

import (
	"io"
	"os"
	"path/filepath"
)

const (
	FlagName = "global-config"
)

// Root represents the configuration directory.
type Root string

func (r Root) Session() File {
	return File(filepath.Join(string(r), "session"))
}

// ReplicaID is a unique identifier for the Coder server.
func (r Root) ReplicaID() File {
	return File(filepath.Join(string(r), "replica_id"))
}

func (r Root) URL() File {
	return File(filepath.Join(string(r), "url"))
}

func (r Root) Organization() File {
	return File(filepath.Join(string(r), "organization"))
}

func (r Root) DotfilesURL() File {
	return File(filepath.Join(string(r), "dotfilesurl"))
}

func (r Root) PostgresPath() string {
	return filepath.Join(string(r), "postgres")
}

func (r Root) PostgresPassword() File {
	return File(filepath.Join(r.PostgresPath(), "password"))
}

func (r Root) PostgresPort() File {
	return File(filepath.Join(r.PostgresPath(), "port"))
}

func (r Root) DeploymentConfigPath() string {
	return filepath.Join(string(r), "server.yaml")
}

// File provides convenience methods for interacting with *os.File.
type File string

// Delete deletes the file.
func (f File) Delete() error {
	return os.Remove(string(f))
}

// Write writes the string to the file.
func (f File) Write(s string) error {
	return write(string(f), 0o600, []byte(s))
}

// Read reads the file to a string.
func (f File) Read() (string, error) {
	byt, err := read(string(f))
	return string(byt), err
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
