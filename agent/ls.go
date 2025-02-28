package agent

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"runtime"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

func (*agent) HandleLS(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var query LSQuery
	if !httpapi.Read(ctx, rw, r, &query) {
		return
	}

	resp, err := listFiles(query)
	if err != nil {
		switch {
		case errors.Is(err, os.ErrNotExist):
			httpapi.Write(ctx, rw, http.StatusNotFound, codersdk.Response{
				Message: "Directory does not exist",
			})
		case errors.Is(err, os.ErrPermission):
			httpapi.Write(ctx, rw, http.StatusForbidden, codersdk.Response{
				Message: "Permission denied",
			})
		default:
			httpapi.Write(ctx, rw, http.StatusInternalServerError, codersdk.Response{
				Message: err.Error(),
			})
		}
		return
	}

	httpapi.Write(ctx, rw, http.StatusOK, resp)
}

func listFiles(query LSQuery) (LSResponse, error) {
	var base string
	switch query.Relativity {
	case LSRelativityHome:
		home, err := os.UserHomeDir()
		if err != nil {
			return LSResponse{}, xerrors.Errorf("failed to get user home directory: %w", err)
		}
		base = home
	case LSRelativityRoot:
		if runtime.GOOS == "windows" {
			// TODO: Eventually, we could have a empty path with a root base
			// return all drives.
			// C drive should be good enough for now.
			base = "C:\\"
		} else {
			base = "/"
		}
	default:
		return LSResponse{}, xerrors.Errorf("unsupported relativity type %q", query.Relativity)
	}

	fullPath := append([]string{base}, query.Path...)
	absolutePathString, err := filepath.Abs(filepath.Join(fullPath...))
	if err != nil {
		return LSResponse{}, xerrors.Errorf("failed to get absolute path: %w", err)
	}

	f, err := os.Open(absolutePathString)
	if err != nil {
		return LSResponse{}, xerrors.Errorf("failed to open directory: %w", err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return LSResponse{}, xerrors.Errorf("failed to stat directory: %w", err)
	}

	if !stat.IsDir() {
		return LSResponse{}, xerrors.New("path is not a directory")
	}

	// `contents` may be partially populated even if the operation fails midway.
	contents, _ := f.Readdir(-1)
	respContents := make([]LSFile, 0, len(contents))
	for _, file := range contents {
		respContents = append(respContents, LSFile{
			Name:               file.Name(),
			AbsolutePathString: filepath.Join(absolutePathString, file.Name()),
			IsDir:              file.IsDir(),
		})
	}

	return LSResponse{
		AbsolutePathString: absolutePathString,
		Contents:           respContents,
	}, nil
}

type LSQuery struct {
	// e.g. [], ["repos", "coder"],
	Path []string `json:"path"`
	// Whether the supplied path is relative to the user's home directory,
	// or the root directory.
	Relativity LSRelativity `json:"relativity"`
}

type LSResponse struct {
	// Returned so clients can display the full path to the user, and
	// copy it to configure file sync
	// e.g. Windows: "C:\\Users\\coder"
	//      Linux: "/home/coder"
	AbsolutePathString string   `json:"absolute_path_string"`
	Contents           []LSFile `json:"contents"`
}

type LSFile struct {
	Name string `json:"name"`
	// e.g. "C:\\Users\\coder\\hello.txt"
	//      "/home/coder/hello.txt"
	AbsolutePathString string `json:"absolute_path_string"`
	IsDir              bool   `json:"is_dir"`
}

type LSRelativity string

const (
	LSRelativityRoot LSRelativity = "root"
	LSRelativityHome LSRelativity = "home"
)
