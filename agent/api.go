package agent

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/go-chi/chi"

	"github.com/coder/coder/coderd/httpapi"
	"github.com/coder/coder/codersdk"
)

func (a *agent) apiHandler() http.Handler {
	r := chi.NewRouter()
	r.Get("/", func(rw http.ResponseWriter, r *http.Request) {
		httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.Response{
			Message: "Hello from the agent!",
		})
	})

	lp := &listeningPortsHandler{}
	r.Get("/api/v0/listening-ports", lp.handler)

	logs := &logsHandler{
		logFiles: []*logFile{
			{
				name: codersdk.WorkspaceAgentLogAgent,
				path: filepath.Join(a.logDir, string(codersdk.WorkspaceAgentLogAgent)),
			},
			{
				name: codersdk.WorkspaceAgentLogStartupScript,
				path: filepath.Join(a.logDir, string(codersdk.WorkspaceAgentLogStartupScript)),
			},
		},
	}
	r.Route("/api/v0/logs", func(r chi.Router) {
		r.Get("/", logs.list)
		r.Route("/{log}", func(r chi.Router) {
			r.Get("/", logs.info)
			r.Get("/tail", logs.tail)
		})
	})

	return r
}

type listeningPortsHandler struct {
	mut   sync.Mutex
	ports []codersdk.WorkspaceAgentListeningPort
	mtime time.Time
}

// handler returns a list of listening ports. This is tested by coderd's
// TestWorkspaceAgentListeningPorts test.
func (lp *listeningPortsHandler) handler(rw http.ResponseWriter, r *http.Request) {
	ports, err := lp.getListeningPorts()
	if err != nil {
		httpapi.Write(r.Context(), rw, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not scan for listening ports.",
			Detail:  err.Error(),
		})
		return
	}

	httpapi.Write(r.Context(), rw, http.StatusOK, codersdk.WorkspaceAgentListeningPortsResponse{
		Ports: ports,
	})
}

type logFile struct {
	name codersdk.WorkspaceAgentLog
	path string

	mu     sync.Mutex // Protects following.
	lines  int
	offset int64
}

type logsHandler struct {
	logFiles []*logFile
}

func (lh *logsHandler) list(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	logs, ok := logFileInfo(w, r, lh.logFiles...)
	if !ok {
		return
	}

	httpapi.Write(ctx, w, http.StatusOK, logs)
}

func (lh *logsHandler) info(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	logName := codersdk.WorkspaceAgentLog(chi.URLParam(r, "log"))
	if logName == "" {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Missing log URL parameter.",
		})
		return
	}

	for _, f := range lh.logFiles {
		if f.name == logName {
			logs, ok := logFileInfo(w, r, f)
			if !ok {
				return
			}

			httpapi.Write(ctx, w, http.StatusOK, logs[0])
			return
		}
	}

	httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
		Message: "Log file not found.",
	})
}

func (lh *logsHandler) tail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	logName := codersdk.WorkspaceAgentLog(chi.URLParam(r, "log"))
	if logName == "" {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message: "Missing log URL parameter.",
		})
		return
	}

	qp := r.URL.Query()
	parser := httpapi.NewQueryParamParser()
	offset := parser.Int(qp, 0, "offset")
	limit := parser.Int(qp, 0, "limit")
	if len(parser.Errors) > 0 {
		httpapi.Write(ctx, w, http.StatusBadRequest, codersdk.Response{
			Message:     "Query parameters have invalid values.",
			Validations: parser.Errors,
		})
		return
	}

	var lf *logFile
	for _, f := range lh.logFiles {
		if f.name == logName {
			lf = f
			break
		}
	}
	if lf == nil {
		httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
			Message: "Log file not found.",
		})
		return
	}

	f, err := os.Open(lf.path)
	if err != nil {
		if os.IsNotExist(err) {
			httpapi.Write(ctx, w, http.StatusNotFound, codersdk.Response{
				Message: "Log file not found.",
			})
			return
		}
		httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
			Message: "Could not open log file.",
			Detail:  err.Error(),
		})
		return
	}
	defer f.Close()

	var lines []string
	fr := bufio.NewReader(f)
	n := -1
	for {
		b, err := fr.ReadBytes('\n')
		if err != nil {
			// Note, we skip incomplete lines with no newline.
			if err == io.EOF {
				break
			}
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Could not read log file.",
				Detail:  err.Error(),
			})
			return
		}
		n++
		if n < offset {
			continue
		}
		b = bytes.TrimRight(b, "\r\n")
		lines = append(lines, string(b))

		if limit > 0 && len(lines) >= limit {
			break
		}
	}

	httpapi.Write(ctx, w, http.StatusOK, codersdk.WorkspaceAgentLogTailResponse{
		Offset: offset,
		Count:  len(lines),
		Lines:  lines,
	})
}

func logFileInfo(w http.ResponseWriter, r *http.Request, lf ...*logFile) ([]codersdk.WorkspaceAgentLogInfo, bool) {
	ctx := r.Context()

	var logs []codersdk.WorkspaceAgentLogInfo
	for _, f := range lf {
		size, lines, modified, exists, err := f.fileInfo()
		if err != nil {
			httpapi.Write(ctx, w, http.StatusInternalServerError, codersdk.Response{
				Message: "Could not gather log file info.",
				Detail:  err.Error(),
			})
			return nil, false
		}

		logs = append(logs, codersdk.WorkspaceAgentLogInfo{
			Name:     f.name,
			Path:     f.path,
			Size:     size,
			Lines:    lines,
			Exists:   exists,
			Modified: modified,
		})
	}

	return logs, true
}

// fileInfo counts the number of lines in the log file and caches
// the logFile's line count and offset.
func (lf *logFile) fileInfo() (size int64, lines int, modified time.Time, exists bool, err error) {
	lf.mu.Lock()
	defer lf.mu.Unlock()

	f, err := os.Open(lf.path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, time.Time{}, false, nil
		}
		return 0, 0, time.Time{}, false, err
	}
	defer f.Close()

	// Note, modified time will not be entirely accurate, but we rather
	// give an old timestamp than one that is newer than when we counted
	// the lines.
	info, err := f.Stat()
	if err != nil {
		return 0, 0, time.Time{}, false, err
	}

	_, err = f.Seek(lf.offset, io.SeekStart)
	if err != nil {
		return 0, 0, time.Time{}, false, err
	}

	r := bufio.NewReader(f)
	for {
		b, err := r.ReadBytes('\n')
		if err != nil {
			// Note, we skip incomplete lines with no newline.
			if err == io.EOF {
				break
			}
			return 0, 0, time.Time{}, false, err
		}
		size += int64(len(b))
		lines++
	}
	lf.offset += size
	lf.lines += lines

	return lf.offset, lf.lines, info.ModTime(), true, nil
}
