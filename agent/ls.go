package agent

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"

	"github.com/shirou/gopsutil/v4/disk"
	"github.com/spf13/afero"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

var WindowsDriveRegex = regexp.MustCompile(`^[a-zA-Z]:\\$`)

func (a *agent) HandleLS(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// An absolute path may be optionally provided, otherwise a path split into an
	// array must be provided in the body (which can be relative).
	query := r.URL.Query()
	parser := httpapi.NewQueryParamParser()
	path := parser.String(query, "", "path")
	parser.ErrorExcessParams(query)
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, rw, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}

	var req workspacesdk.LSRequest
	if !httpapi.Read(ctx, rw, r, &req) {
		return
	}

	resp, err := listFiles(a.filesystem, path, req)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case errors.Is(err, os.ErrNotExist):
			status = http.StatusNotFound
		case errors.Is(err, os.ErrPermission):
			status = http.StatusForbidden
		default:
		}
		httpapi.Write(ctx, rw, status, codersdk.Response{
			Message: err.Error(),
		})
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func listFiles(fs afero.Fs, path string, query workspacesdk.LSRequest) (workspacesdk.LSResponse, error) {
	absolutePathString := path
	if absolutePathString != "" {
		if !filepath.IsAbs(path) {
			return workspacesdk.LSResponse{}, xerrors.Errorf("path must be absolute: %q", path)
		}
	} else {
		var fullPath []string
		switch query.Relativity {
		case workspacesdk.LSRelativityHome:
			home, err := os.UserHomeDir()
			if err != nil {
				return workspacesdk.LSResponse{}, xerrors.Errorf("failed to get user home directory: %w", err)
			}
			fullPath = []string{home}
		case workspacesdk.LSRelativityRoot:
			if runtime.GOOS == "windows" {
				if len(query.Path) == 0 {
					return listDrives()
				}
				if !WindowsDriveRegex.MatchString(query.Path[0]) {
					return workspacesdk.LSResponse{}, xerrors.Errorf("invalid drive letter %q", query.Path[0])
				}
			} else {
				fullPath = []string{"/"}
			}
		default:
			return workspacesdk.LSResponse{}, xerrors.Errorf("unsupported relativity type %q", query.Relativity)
		}

		fullPath = append(fullPath, query.Path...)
		fullPathRelative := filepath.Join(fullPath...)
		var err error
		absolutePathString, err = filepath.Abs(fullPathRelative)
		if err != nil {
			return workspacesdk.LSResponse{}, xerrors.Errorf("failed to get absolute path of %q: %w", fullPathRelative, err)
		}
	}

	// codeql[go/path-injection] - The intent is to allow the user to navigate to any directory in their workspace.
	f, err := fs.Open(absolutePathString)
	if err != nil {
		return workspacesdk.LSResponse{}, xerrors.Errorf("failed to open directory %q: %w", absolutePathString, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return workspacesdk.LSResponse{}, xerrors.Errorf("failed to stat directory %q: %w", absolutePathString, err)
	}

	if !stat.IsDir() {
		return workspacesdk.LSResponse{}, xerrors.Errorf("path %q is not a directory", absolutePathString)
	}

	// `contents` may be partially populated even if the operation fails midway.
	contents, _ := f.Readdir(-1)
	respContents := make([]workspacesdk.LSFile, 0, len(contents))
	for _, file := range contents {
		respContents = append(respContents, workspacesdk.LSFile{
			Name:               file.Name(),
			AbsolutePathString: filepath.Join(absolutePathString, file.Name()),
			IsDir:              file.IsDir(),
		})
	}

	// Sort alphabetically: directories then files
	slices.SortFunc(respContents, func(a, b workspacesdk.LSFile) int {
		if a.IsDir && !b.IsDir {
			return -1
		}
		if !a.IsDir && b.IsDir {
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})

	absolutePath := pathToArray(absolutePathString)

	return workspacesdk.LSResponse{
		AbsolutePath:       absolutePath,
		AbsolutePathString: absolutePathString,
		Contents:           respContents,
	}, nil
}

func listDrives() (workspacesdk.LSResponse, error) {
	// disk.Partitions() will return partitions even if there was a failure to
	// get one. Any errored partitions will not be returned.
	partitionStats, err := disk.Partitions(true)
	if err != nil && len(partitionStats) == 0 {
		// Only return the error if there were no partitions returned.
		return workspacesdk.LSResponse{}, xerrors.Errorf("failed to get partitions: %w", err)
	}

	contents := make([]workspacesdk.LSFile, 0, len(partitionStats))
	for _, a := range partitionStats {
		// Drive letters on Windows have a trailing separator as part of their name.
		// i.e. `os.Open("C:")` does not work, but `os.Open("C:\\")` does.
		name := a.Mountpoint + string(os.PathSeparator)
		contents = append(contents, workspacesdk.LSFile{
			Name:               name,
			AbsolutePathString: name,
			IsDir:              true,
		})
	}

	return workspacesdk.LSResponse{
		AbsolutePath:       []string{},
		AbsolutePathString: "",
		Contents:           contents,
	}, nil
}

func pathToArray(path string) []string {
	out := strings.FieldsFunc(path, func(r rune) bool {
		return r == os.PathSeparator
	})
	// Drive letters on Windows have a trailing separator as part of their name.
	// i.e. `os.Open("C:")` does not work, but `os.Open("C:\\")` does.
	if runtime.GOOS == "windows" && len(out) > 0 {
		out[0] += string(os.PathSeparator)
	}
	return out
}
