package archivefs

import (
	"bytes"
	"io"
	"io/fs"
)

type FS struct {
	files map[string]*File
}

// FS implements fs.FS
var _ fs.ReadDirFS = &FS{}

func (f *FS) Open(fileName string) (fs.File, error) {
	file := f.files[fileName]
	if file == nil {
		// Check if it's a directory

		return nil, fs.ErrNotExist
	}
	openFile := OpenFile{
		info:   file.info,
		Reader: bytes.NewReader(file.content),
	}
	return &openFile, nil
}

func (f *FS) ReadDir(dirName string) ([]fs.DirEntry

type File struct {
	info    fs.FileInfo
	content []byte
}

type OpenFile struct {
	info fs.FileInfo
	io.Reader
}

// OpenFile implements fs.File
var _ fs.File = &OpenFile{}

func (o *OpenFile) Stat() (fs.FileInfo, error) {
	return o.info, nil
}

func (o *OpenFile) Read(p []byte) (int, error) {
	return o.Reader.Read(p)
}

func (o *OpenFile) Close() error {
	o.Reader = nil
	return nil
}
