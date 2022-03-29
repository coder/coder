package examples

import (
	"archive/tar"
	"bytes"
	"embed"
	"path"
	"sync"

	"github.com/gohugoio/hugo/parser/pageparser"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"
)

var (
	//go:embed */*.md
	//go:embed */*.tf
	files embed.FS

	examples      = make([]Example, 0)
	parseExamples sync.Once
	archives      = singleflight.Group{}
)

type Example struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Markdown    string `json:"markdown"`
}

// List returns all embedded examples.
func List() ([]Example, error) {
	var returnError error
	parseExamples.Do(func() {
		dirs, err := files.ReadDir(".")
		if err != nil {
			returnError = xerrors.Errorf("read dir: %w", err)
			return
		}

		for _, dir := range dirs {
			exampleID := dir.Name()
			// Each one of these is a example!
			readme, err := files.ReadFile(path.Join(dir.Name(), "README.md"))
			if err != nil {
				returnError = xerrors.Errorf("example %q does not contain README.md", exampleID)
				return
			}

			frontMatter, err := pageparser.ParseFrontMatterAndContent(bytes.NewReader(readme))
			if err != nil {
				returnError = xerrors.Errorf("parse example %q front matter: %w", exampleID, err)
				return
			}

			nameRaw, exists := frontMatter.FrontMatter["name"]
			if !exists {
				returnError = xerrors.Errorf("example %q front matter does not contain name", exampleID)
				return
			}

			name, valid := nameRaw.(string)
			if !valid {
				returnError = xerrors.Errorf("example %q name isn't a string", exampleID)
				return
			}

			descriptionRaw, exists := frontMatter.FrontMatter["description"]
			if !exists {
				returnError = xerrors.Errorf("example %q front matter does not contain name", exampleID)
				return
			}

			description, valid := descriptionRaw.(string)
			if !valid {
				returnError = xerrors.Errorf("example %q description isn't a string", exampleID)
				return
			}

			examples = append(examples, Example{
				ID:          exampleID,
				Name:        name,
				Description: description,
				Markdown:    string(frontMatter.Content),
			})
		}
	})
	return examples, returnError
}

// Archive returns a tar by example ID.
func Archive(exampleID string) ([]byte, error) {
	rawData, err, _ := archives.Do(exampleID, func() (interface{}, error) {
		examples, err := List()
		if err != nil {
			return nil, xerrors.Errorf("list: %w", err)
		}

		var selected Example
		for _, example := range examples {
			if example.ID != exampleID {
				continue
			}
			selected = example
			break
		}

		if selected.ID == "" {
			return nil, xerrors.Errorf("example with id %q not found", exampleID)
		}

		entries, err := files.ReadDir(exampleID)
		if err != nil {
			return nil, xerrors.Errorf("read dir: %w", err)
		}

		var buffer bytes.Buffer
		tarWriter := tar.NewWriter(&buffer)

		for _, entry := range entries {
			file, err := files.Open(path.Join(exampleID, entry.Name()))
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
