package chatd

import (
	"encoding/json"
	"path"
	"slices"
	"strings"

	"charm.land/fantasy"

	"github.com/coder/coder/v2/coderd/x/chatd/chattool"
)

// agentPath normalizes a tool or agent path so the POSIX path package can
// reason about it: a drive path's backslashes become forward slashes, which
// Windows accepts; on POSIX a backslash is an ordinary name character.
func agentPath(p string) string {
	if isDrivePath(p) {
		p = strings.ReplaceAll(p, "\\", "/")
	}
	return path.Clean(p)
}

// isDrivePath reports whether p starts with a Windows drive letter such as C:.
func isDrivePath(p string) bool {
	return len(p) >= 2 && p[1] == ':'
}

// pathKey is the comparison form of a normalized path: drive paths compare
// case-folded, since Windows file systems are case-insensitive by default.
func pathKey(p string) string {
	if isDrivePath(p) {
		return strings.ToLower(p)
	}
	return p
}

// isAbsAgentPath reports whether the agent's file system takes p as
// absolute: a drive root such as C:/ on Windows, a leading slash elsewhere.
// The agent rejects a whole probe batch over one path it deems relative.
func isAbsAgentPath(p, operatingSystem string) bool {
	if operatingSystem == "windows" {
		return len(p) >= 3 && p[1] == ':' && p[2] == '/'
	}
	return path.IsAbs(p)
}

// isUNCPath reports whether raw is a Windows network path such as
// \\server\share, which agentPath would fold into a POSIX root that the
// Windows agent rejects as relative.
func isUNCPath(raw string) bool {
	return len(raw) >= 2 && (raw[0] == '\\' || raw[0] == '/') && (raw[1] == '\\' || raw[1] == '/')
}

func isRootAgentPath(dir string) bool {
	return dir == "/" || dir == "." || (len(dir) == 2 && dir[1] == ':')
}

// touchedPaths returns the paths the executed tool calls addressed, as the
// tools received them: file tool paths and explicit execute workdirs. The
// default execute directory is already a scan root.
func touchedPaths(calls []fantasy.ToolCallContent, results []fantasy.Content) (files, dirs []string) {
	executed := make(map[string]struct{}, len(results))
	for _, block := range results {
		if result, ok := fantasy.AsContentType[fantasy.ToolResultContent](block); ok {
			executed[result.ToolCallID] = struct{}{}
		} else if result, ok := fantasy.AsContentType[*fantasy.ToolResultContent](block); ok && result != nil {
			executed[result.ToolCallID] = struct{}{}
		}
	}
	addFile := func(p string) {
		if p = strings.TrimSpace(p); p != "" {
			files = append(files, p)
		}
	}
	for _, call := range calls {
		if _, ok := executed[call.ToolCallID]; !ok {
			continue
		}
		switch call.ToolName {
		case chattool.ReadFileToolName, chattool.WriteFileToolName:
			var args struct {
				Path string `json:"path"`
			}
			if json.Unmarshal([]byte(call.Input), &args) == nil {
				addFile(args.Path)
			}
		case chattool.EditFilesToolName:
			var args struct {
				Files []struct {
					Path string `json:"path"`
				} `json:"files"`
			}
			if json.Unmarshal([]byte(call.Input), &args) == nil {
				for _, file := range args.Files {
					addFile(file.Path)
				}
			}
		case chattool.ExecuteToolName:
			var args struct {
				WorkDir *string `json:"workdir"`
			}
			if json.Unmarshal([]byte(call.Input), &args) == nil && args.WorkDir != nil && *args.WorkDir != "" {
				dirs = append(dirs, *args.WorkDir)
			}
		}
	}
	return files, dirs
}

// agentTouchedPaths normalizes touched paths for the agent's file system
// and drops the ones the agent would reject as relative, including Windows
// network paths, which would fail the whole probe batch.
func agentTouchedPaths(files, dirs []string, operatingSystem string) (outFiles, outDirs []string) {
	keep := func(out []string, p string) []string {
		if operatingSystem == "windows" && isUNCPath(p) {
			return out
		}
		if p = agentPath(p); isAbsAgentPath(p, operatingSystem) {
			out = append(out, p)
		}
		return out
	}
	for _, file := range files {
		outFiles = keep(outFiles, file)
	}
	for _, dir := range dirs {
		outDirs = keep(outDirs, dir)
	}
	return outFiles, outDirs
}

// candidateInstructionDirs lists each touched directory and its ancestors
// below the working directory, whose own files arrive through the snapshot,
// deduplicated and shallowest first so the request cap keeps the widest scopes.
func candidateInstructionDirs(files, dirs []string, workingDir string) []string {
	workingKey := pathKey(agentPath(workingDir))
	seen := make(map[string]struct{})
	var out []string
	visit := func(dir string) {
		for {
			key := pathKey(dir)
			if isRootAgentPath(dir) || key == workingKey || (workingKey != "/" && strings.HasPrefix(workingKey+"/", key+"/")) {
				return
			}
			if _, ok := seen[key]; ok {
				return
			}
			seen[key] = struct{}{}
			out = append(out, dir)
			dir = path.Dir(dir)
		}
	}
	for _, file := range files {
		visit(path.Dir(file))
	}
	for _, dir := range dirs {
		visit(dir)
	}
	sortShallowestFirst(out)
	return out
}

// sortShallowestFirst orders directories by depth, then name, so caps and
// budgets applied in order favor the files that govern the most paths.
func sortShallowestFirst(dirs []string) {
	slices.SortFunc(dirs, func(a, b string) int {
		if da, db := strings.Count(a, "/"), strings.Count(b, "/"); da != db {
			return da - db
		}
		return strings.Compare(a, b)
	})
}
