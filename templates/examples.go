package examples

import (
	"archive/tar"
	"bytes"
	"embed"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gohugoio/hugo/parser/pageparser"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"
)

var (
	//go:embed quickstart
	files embed.FS

	exampleBasePath = "https://github.com/coder/coder/tree/main/examples"
	examples        = make([]Example, 0)
	parseExamples   sync.Once
	archives        = singleflight.Group{}
)

type Example struct {
	ID            string `json:"id"`
	URL           string `json:"url"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	Markdown      string `json:"markdown"`
	DirectoryPath string `json:"directory_path"`
}

const rootDir = "quickstart"

// WalkForExamples will walk recursively through the examples directory and call
// exampleDirectory() on each directory that contains a main.tf file.
func WalkForExamples(files fs.FS, rootDir string, exampleDirectory func(path string)) error {
	return walkDir(exampleDirectory, files, rootDir)
}

func walkDir(exampleDirectory func(path string), fileFS fs.FS, path string) error {
	file, err := fileFS.Open(path)
	if err != nil {
		return xerrors.Errorf("open file %q: %w", path, err)
	}
	info, err := file.Stat()
	if err != nil {
		return xerrors.Errorf("stat file %q: %w", path, err)
	}

	if info.IsDir() {
		files, err := fs.ReadDir(files, path)
		if err != nil {
			return xerrors.Errorf("read dir %q: %w", path, err)
		}

		for _, file := range files {
			if strings.EqualFold(file.Name(), "main.tf") {
				// This is an example dir
				exampleDirectory(path)
				return nil
			} else if file.IsDir() {
				err := walkDir(exampleDirectory, fileFS, filepath.Join(path, file.Name()))
				if err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// List returns all embedded examples.
func List() ([]Example, error) {
	var returnError error
	parseExamples.Do(func() {
		err := WalkForExamples(files, rootDir, func(path string) {
			exampleID := filepath.Base(path)
			exampleURL := exampleBasePath + exampleID
			// Each one of these is a example!
			readme, err := fs.ReadFile(files, filepath.Join(path, "README.md"))
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
				ID:            exampleID,
				URL:           exampleURL,
				Name:          name,
				Description:   description,
				Markdown:      string(frontMatter.Content),
				DirectoryPath: path,
			})
		})
		if err != nil {
			returnError = xerrors.Errorf("walking embedded files: %w", err)
			return
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
			if example.ID == exampleID {
				selected = example
				break
			}
		}

		if selected.ID == "" {
			return nil, xerrors.Errorf("example with id %q not found", exampleID)
		}

		exampleFiles, err := fs.Sub(files, selected.DirectoryPath)
		if err != nil {
			return nil, xerrors.Errorf("get example fs: %w", err)
		}

		var buffer bytes.Buffer
		tarWriter := tar.NewWriter(&buffer)

		err = fs.WalkDir(exampleFiles, ".", func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}

			info, err := entry.Info()
			if err != nil {
				return xerrors.Errorf("stat file: %w", err)
			}

			header, err := tar.FileInfoHeader(info, entry.Name())
			if err != nil {
				return xerrors.Errorf("get file header: %w", err)
			}
			header.Mode = 0644

			if entry.IsDir() {
				header.Name = path + "/"

				err = tarWriter.WriteHeader(header)
				if err != nil {
					return xerrors.Errorf("write file: %w", err)
				}
			} else {
				header.Name = path

				file, err := exampleFiles.Open(path)
				if err != nil {
					return xerrors.Errorf("open file %s: %w", path, err)
				}
				defer file.Close()

				err = tarWriter.WriteHeader(header)
				if err != nil {
					return xerrors.Errorf("write file: %w", err)
				}

				_, err = io.Copy(tarWriter, file)
				if err != nil {
					return xerrors.Errorf("write: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			return nil, xerrors.Errorf("walk example directory: %w", err)
		}

		err = tarWriter.Close()
		if err != nil {
			return nil, xerrors.Errorf("close archive: %w", err)
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
