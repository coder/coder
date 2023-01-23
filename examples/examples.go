package examples

import (
	"archive/tar"
	"bytes"
	"embed"
	"io"
	"io/fs"
	"path"
	"strings"
	"sync"

	"github.com/gohugoio/hugo/parser/pageparser"
	"golang.org/x/sync/singleflight"
	"golang.org/x/xerrors"

	"github.com/coder/coder/codersdk"
)

var (
	// Only some templates are embedded that we want to display inside the UI.
	//go:embed templates/aws-ecs-container
	//go:embed templates/aws-linux
	//go:embed templates/aws-windows
	//go:embed templates/azure-linux
	//go:embed templates/do-linux
	//go:embed templates/docker
	//go:embed templates/docker-code-server
	//go:embed templates/docker-image-builds
	//go:embed templates/docker-with-dotfiles
	//go:embed templates/gcp-linux
	//go:embed templates/gcp-vm-container
	//go:embed templates/gcp-windows
	//go:embed templates/kubernetes
	files embed.FS

	exampleBasePath = "https://github.com/coder/coder/tree/main/examples/templates/"
	examples        = make([]codersdk.TemplateExample, 0)
	parseExamples   sync.Once
	archives        = singleflight.Group{}
	ErrNotFound     = xerrors.New("example not found")
)

const rootDir = "templates"

// List returns all embedded examples.
func List() ([]codersdk.TemplateExample, error) {
	var returnError error
	parseExamples.Do(func() {
		files, err := fs.Sub(files, rootDir)
		if err != nil {
			returnError = xerrors.Errorf("get example fs: %w", err)
		}

		dirs, err := fs.ReadDir(files, ".")
		if err != nil {
			returnError = xerrors.Errorf("read dir: %w", err)
			return
		}

		for _, dir := range dirs {
			if !dir.IsDir() {
				continue
			}
			exampleID := dir.Name()
			exampleURL := exampleBasePath + exampleID
			// Each one of these is a example!
			readme, err := fs.ReadFile(files, path.Join(dir.Name(), "README.md"))
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

			tags := []string{}
			tagsRaw, exists := frontMatter.FrontMatter["tags"]
			if exists {
				tagsI, valid := tagsRaw.([]interface{})
				if !valid {
					returnError = xerrors.Errorf("example %q tags isn't a slice: type %T", exampleID, tagsRaw)
					return
				}
				for _, tagI := range tagsI {
					tag, valid := tagI.(string)
					if !valid {
						returnError = xerrors.Errorf("example %q tag isn't a string: type %T", exampleID, tagI)
						return
					}
					tags = append(tags, tag)
				}
			}

			var icon string
			iconRaw, exists := frontMatter.FrontMatter["icon"]
			if exists {
				icon, valid = iconRaw.(string)
				if !valid {
					returnError = xerrors.Errorf("example %q icon isn't a string", exampleID)
					return
				}
			}

			examples = append(examples, codersdk.TemplateExample{
				ID:          exampleID,
				URL:         exampleURL,
				Name:        name,
				Description: description,
				Icon:        icon,
				Tags:        tags,
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

		var selected codersdk.TemplateExample
		for _, example := range examples {
			if example.ID != exampleID {
				continue
			}
			selected = example
			break
		}

		if selected.ID == "" {
			return nil, xerrors.Errorf("example with id %q not found: %w", exampleID, ErrNotFound)
		}

		exampleFiles, err := fs.Sub(files, path.Join(rootDir, exampleID))
		if err != nil {
			return nil, xerrors.Errorf("get example fs: %w", err)
		}

		var buffer bytes.Buffer
		tarWriter := tar.NewWriter(&buffer)

		err = fs.WalkDir(exampleFiles, ".", func(path string, entry fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if path == "." {
				// Tar files don't have a root directory.
				return nil
			}

			info, err := entry.Info()
			if err != nil {
				return xerrors.Errorf("stat file: %w", err)
			}

			header, err := tar.FileInfoHeader(info, "")
			if err != nil {
				return xerrors.Errorf("get file header: %w", err)
			}
			header.Name = strings.TrimPrefix(path, "./")
			header.Mode = 0644

			if entry.IsDir() {
				// Trailing slash on entry name is not required. Our tar
				// creation code for tarring up a local directory doesn't
				// include slashes so this we don't include them here for
				// consistency.
				// header.Name += "/"
				header.Mode = 0755
				header.Typeflag = tar.TypeDir
				err = tarWriter.WriteHeader(header)
				if err != nil {
					return xerrors.Errorf("write file: %w", err)
				}
			} else {
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
