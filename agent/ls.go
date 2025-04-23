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
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk"
)

var WindowsDriveRegex = regexp.MustCompile(`^[a-zA-Z]:\\$`)

func (*agent) HandleLS(rw http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var query LSRequest
	if !httpapi.Read(ctx, rw, r, &query) {
		return
	}

	resp, err := listFiles(query)
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

func listFiles(query LSRequest) (LSResponse, error) {
	var fullPath []string
	switch query.Relativity {
	case LSRelativityHome:
		home, err := os.UserHomeDir()
		if err != nil {
			return LSResponse{}, xerrors.Errorf("failed to get user home directory: %w", err)
		}
		fullPath = []string{home}
	case LSRelativityRoot:
		if runtime.GOOS == "windows" {
			if len(query.Path) == 0 {
				return listDrives()
			}
			if !WindowsDriveRegex.MatchString(query.Path[0]) {
				return LSResponse{}, xerrors.Errorf("invalid drive letter %q", query.Path[0])
			}
		} else {
			fullPath = []string{"/"}
		}
	default:
		return LSResponse{}, xerrors.Errorf("unsupported relativity type %q", query.Relativity)
	}

	fullPath = append(fullPath, query.Path...)
	fullPathRelative := filepath.Join(fullPath...)
	absolutePathString, err := filepath.Abs(fullPathRelative)
	if err != nil {
		return LSResponse{}, xerrors.Errorf("failed to get absolute path of %q: %w", fullPathRelative, err)
	}

	// codeql[go/path-injection] - The intent is to allow the user to navigate to any directory in their workspace.
	f, err := os.Open(absolutePathString)
	if err != nil {
		return LSResponse{}, xerrors.Errorf("failed to open directory %q: %w", absolutePathString, err)
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return LSResponse{}, xerrors.Errorf("failed to stat directory %q: %w", absolutePathString, err)
	}

	if !stat.IsDir() {
		return LSResponse{}, xerrors.Errorf("path %q is not a directory", absolutePathString)
	}

	// `contents` may be partially populated even if the operation fails midway.
	contents, _ := f.ReadDir(-1)
	respContents := make([]LSFile, 0, len(contents))
	for _, file := range contents {
		respContents = append(respContents, LSFile{
			Name:               file.Name(),
			AbsolutePathString: filepath.Join(absolutePathString, file.Name()),
			IsDir:              file.IsDir(),
		})
	}

	// Sort alphabetically: directories then files
	slices.SortFunc(respContents, func(a, b LSFile) int {
		if a.IsDir && !b.IsDir {
			return -1
		}
		if !a.IsDir && b.IsDir {
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})

	absolutePath := pathToArray(absolutePathString)

	return LSResponse{
		AbsolutePath:       absolutePath,
		AbsolutePathString: absolutePathString,
		Contents:           respContents,
	}, nil
}

func listDrives() (LSResponse, error) {
	// disk.Partitions() will return partitions even if there was a failure to
	// get one. Any errored partitions will not be returned.
	partitionStats, err := disk.Partitions(true)
	if err != nil && len(partitionStats) == 0 {
		// Only return the error if there were no partitions returned.
		return LSResponse{}, xerrors.Errorf("failed to get partitions: %w", err)
	}

	contents := make([]LSFile, 0, len(partitionStats))
	for _, a := range partitionStats {
		// Drive letters on Windows have a trailing separator as part of their name.
		// i.e. `os.Open("C:")` does not work, but `os.Open("C:\\")` does.
		name := a.Mountpoint + string(os.PathSeparator)
		contents = append(contents, LSFile{
			Name:               name,
			AbsolutePathString: name,
			IsDir:              true,
		})
	}

	return LSResponse{
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

type LSRequest struct {
	// e.g. [], ["repos", "coder"],
	Path []string `json:"path"`
	// Whether the supplied path is relative to the user's home directory,
	// or the root directory.
	Relativity LSRelativity `json:"relativity"`
}

type LSResponse struct {
	AbsolutePath []string `json:"absolute_path"`
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
