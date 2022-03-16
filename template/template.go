package template

import (
	"archive/tar"
	"bytes"
	"embed"
	"path"
	"sync"

	"github.com/gohugoio/hugo/parser/pageparser"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

var (
	//go:embed */*.md
	//go:embed */*.tf
	files embed.FS

	templates      = make([]codersdk.Template, 0)
	parseTemplates sync.Once
	archives       = singleflight.Group{}
)

// Parses templates from the embedded archive and inserts them into the map.
func init() {

}

// List returns all embedded templates.
func List() ([]codersdk.Template, error) {
	var returnError error
	parseTemplates.Do(func() {
		dirs, err := files.ReadDir(".")
		if err != nil {
			returnError = xerrors.Errorf("read dir: %w", err)
			return
		}
		for _, dir := range dirs {
			templateID := dir.Name()
			// Each one of these is a template!
			readme, err := files.ReadFile(path.Join(dir.Name(), "README.md"))
			if err != nil {
				returnError = xerrors.Errorf("template %q does not contain README.md", templateID)
				return
			}
			frontMatter, err := pageparser.ParseFrontMatterAndContent(bytes.NewReader(readme))
			if err != nil {
				returnError = xerrors.Errorf("parse template %q front matter: %w", templateID, err)
				return
			}
			nameRaw, exists := frontMatter.FrontMatter["name"]
			if !exists {
				returnError = xerrors.Errorf("template %q front matter does not contain name", templateID)
				return
			}
			name, valid := nameRaw.(string)
			if !valid {
				returnError = xerrors.Errorf("template %q name isn't a string", templateID)
				return
			}
			descriptionRaw, exists := frontMatter.FrontMatter["description"]
			if !exists {
				returnError = xerrors.Errorf("template %q front matter does not contain name", templateID)
				return
			}
			description, valid := descriptionRaw.(string)
			if !valid {
				returnError = xerrors.Errorf("template %q description isn't a string", templateID)
				return
			}
			templates = append(templates, codersdk.Template{
				ID:          templateID,
				Name:        name,
				Description: description,
				Markdown:    string(frontMatter.Content),
			})
		}
	})
	return templates, returnError
}

// Archive returns a tar by template ID.
func Archive(templateID string) ([]byte, error) {
	rawData, err, _ := archives.Do(templateID, func() (interface{}, error) {
		templates, err := List()
		if err != nil {
			return nil, err
		}
		var selected codersdk.Template
		for _, template := range templates {
			if template.ID != templateID {
				continue
			}
			selected = template
			break
		}
		if selected.ID == "" {
			return nil, xerrors.Errorf("template with id %q not found", templateID)
		}

		entries, err := files.ReadDir(templateID)
		if err != nil {
			return nil, xerrors.Errorf("read dir: %w", err)
		}

		var buffer bytes.Buffer
		tarWriter := tar.NewWriter(&buffer)

		for _, entry := range entries {
			file, err := files.Open(path.Join(templateID, entry.Name()))
			if err != nil {
				return nil, xerrors.Errorf("open file: %w", err)
			}
			info, err := file.Stat()
			if err != nil {
				return nil, xerrors.Errorf("stat file: %w", err)
			}
			if info.IsDir() {
				continue
			}
			data := make([]byte, info.Size())
			_, err = file.Read(data)
			if err != nil {
				return nil, xerrors.Errorf("read data: %w", err)
			}
			header, err := tar.FileInfoHeader(info, entry.Name())
			if err != nil {
				return nil, xerrors.Errorf("get file header: %w", err)
			}
			header.Mode = 0644
			err = tarWriter.WriteHeader(header)
			if err != nil {
				return nil, xerrors.Errorf("write file: %w", err)
			}
			_, err = tarWriter.Write(data)
			if err != nil {
				return nil, xerrors.Errorf("write: %w", err)
			}
		}
		err = tarWriter.Flush()
		if err != nil {
			return nil, xerrors.Errorf("flush archive: %w", err)
		}
		return buffer.Bytes(), nil
	})
	if err != nil {
		return nil, err
	}
	data, valid := rawData.([]byte)
	if !valid {
		panic("dev error: data must be a byte slice")
	}
	return data, nil
}
