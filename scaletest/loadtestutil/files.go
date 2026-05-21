package loadtestutil

import (
	"archive/tar"
	"bytes"
	"path/filepath"
	"slices"
)

func CreateTarFromFiles(files map[string][]byte) ([]byte, error) {
	buf := new(bytes.Buffer)
	writer := tar.NewWriter(buf)
	dirs := []string{}
	for name, content := range files {
		// We need to add directories before any files that use them. But, we only need to do this
		// once.
		dir := filepath.Dir(name)
		if dir != "." && !slices.Contains(dirs, dir) {
			dirs = append(dirs, dir)
			err := writer.WriteHeader(&tar.Header{
				Name:     dir,
				Mode:     0o755,
				Typeflag: tar.TypeDir,
			})
			if err != nil {
				return nil, err
			}
		}

		err := writer.WriteHeader(&tar.Header{
			Name: name,
			Size: int64(len(content)),
			Mode: 0o644,
		})
		if err != nil {
			return nil, err
		}

		_, err = writer.Write(content)
		if err != nil {
			return nil, err
		}
	}
	// `writer.Close()` function flushes the writer buffer, and adds extra padding to create a legal tarball.
	err := writer.Close()
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
