//go:build !slim
// +build !slim

package template

import (
	"archive/tar"
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"path"

	"github.com/coder/coder/codersdk"
)

var (
	//go:embed */*.json
	//go:embed */*.tf
	files embed.FS

	list     = make([]codersdk.Template, 0)
	archives = map[string][]byte{}
)

// Parses templates from the embedded archive and inserts them into the map.
func init() {
	dirs, err := files.ReadDir(".")
	if err != nil {
		panic(err)
	}
	for _, dir := range dirs {
		// Each one of these is a template!
		templateData, err := files.ReadFile(path.Join(dir.Name(), dir.Name()+".json"))
		if err != nil {
			panic(fmt.Sprintf("template %q does not contain compiled json: %s", dir.Name(), err))
		}
		templateSrc, err := files.ReadFile(path.Join(dir.Name(), dir.Name()+".tf"))
		if err != nil {
			panic(fmt.Sprintf("template %q does not contain terraform source: %s", dir.Name(), err))
		}

		var template codersdk.Template
		err = json.Unmarshal(templateData, &template)
		if err != nil {
			panic(fmt.Sprintf("unmarshal template %q: %s", dir.Name(), err))
		}

		var buffer bytes.Buffer
		tarWriter := tar.NewWriter(&buffer)
		err = tarWriter.WriteHeader(&tar.Header{
			Typeflag: tar.TypeReg,
			Name:     dir.Name() + ".tf",
			Size:     int64(len(templateSrc)),
		})
		if err != nil {
			panic(err)
		}
		_, err = tarWriter.Write(templateSrc)
		if err != nil {
			panic(err)
		}
		err = tarWriter.Flush()
		if err != nil {
			panic(err)
		}
		archives[dir.Name()] = buffer.Bytes()
		list = append(list, template)
	}
}

// List returns all embedded templates.
func List() []codersdk.Template {
	return list
}

// Archive returns a tar by template ID.
func Archive(id string) ([]byte, bool) {
	data, exists := archives[id]
	return data, exists
}
