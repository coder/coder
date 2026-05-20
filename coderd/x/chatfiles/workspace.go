package chatfiles

import (
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/xerrors"
)

const (
	// MaxWorkspaceUploadFileNameBytes matches the stored attachment name limit.
	MaxWorkspaceUploadFileNameBytes = MaxStoredFileNameBytes

	// WorkspaceChatsDir is the directory under the agent's home directory
	// that holds per-chat coder artifacts.
	WorkspaceChatsDir = ".coder/chats"

	// WorkspaceUploadFilesSubdir is the subdirectory of a chat's
	// workspace folder that holds uploaded files.
	WorkspaceUploadFilesSubdir = "files"
)

// ErrWorkspaceUploadNameRequired indicates that a workspace upload name
// is empty after sanitization.
var ErrWorkspaceUploadNameRequired = xerrors.New("workspace upload file name is required")

// SanitizeWorkspaceUploadName extracts a basename, normalizes unsafe
// whitespace, control characters, and Windows-invalid punctuation to
// underscores, strips edge dots, and truncates the result to
// MaxWorkspaceUploadFileNameBytes. It returns
// ErrWorkspaceUploadNameRequired when no safe name remains.
func SanitizeWorkspaceUploadName(name string) (string, error) {
	name = strings.ReplaceAll(strings.TrimSpace(name), `\`, "/")
	name = path.Base(name)
	if name == "." || name == "/" {
		return "", ErrWorkspaceUploadNameRequired
	}
	name = replaceUnsafeWorkspaceUploadRunes(name)
	name = strings.Trim(name, ".")
	name = truncateUTF8Bytes(name, MaxWorkspaceUploadFileNameBytes)
	name = strings.Trim(name, ".")
	if name == "" || name == "_" || isWindowsReservedUploadName(name) {
		return "", ErrWorkspaceUploadNameRequired
	}
	return name, nil
}

func replaceUnsafeWorkspaceUploadRunes(name string) string {
	var b strings.Builder
	lastUnsafe := false
	for _, r := range name {
		if isUnsafeWorkspaceUploadRune(r) {
			if !lastUnsafe {
				_, _ = b.WriteString("_")
				lastUnsafe = true
			}
			continue
		}
		_, _ = b.WriteRune(r)
		lastUnsafe = false
	}
	return b.String()
}

func isUnsafeWorkspaceUploadRune(r rune) bool {
	if unicode.IsSpace(r) || unicode.IsControl(r) || unicode.In(r, unicode.Cf) {
		return true
	}
	switch r {
	case '<', '>', ':', '"', '|', '?', '*':
		return true
	default:
		return false
	}
}

func isWindowsReservedUploadName(name string) bool {
	stem, _, _ := strings.Cut(name, ".")
	stem = strings.ToUpper(stem)
	if stem == "CON" || stem == "PRN" || stem == "AUX" || stem == "NUL" {
		return true
	}
	runes := []rune(stem)
	if len(runes) != 4 {
		return false
	}
	suffix := runes[3]
	if !isWindowsReservedDeviceSuffix(suffix) {
		return false
	}
	return strings.HasPrefix(stem, "COM") || strings.HasPrefix(stem, "LPT")
}

func isWindowsReservedDeviceSuffix(suffix rune) bool {
	return (suffix >= '1' && suffix <= '9') || suffix == '¹' || suffix == '²' || suffix == '³'
}

// WorkspaceChatDir returns the per-chat directory under the agent home directory.
func WorkspaceChatDir(homeDir, chatID string) string {
	return filepath.Join(homeDir, WorkspaceChatsDir, chatID)
}

// WorkspaceUploadDir returns the per-chat workspace upload directory.
func WorkspaceUploadDir(homeDir, chatID string) string {
	return filepath.Join(WorkspaceChatDir(homeDir, chatID), WorkspaceUploadFilesSubdir)
}

// AddCollisionSuffix inserts a `_<n>` suffix before the extension when n > 1.
func AddCollisionSuffix(name string, n int) string {
	if n <= 1 {
		return name
	}
	suffix := "_" + strconv.Itoa(n)
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		stem = name
		ext = ""
	}

	stemBytes := MaxWorkspaceUploadFileNameBytes - len(suffix) - len(ext)
	if stemBytes <= 0 {
		return truncateUTF8Bytes(name, MaxWorkspaceUploadFileNameBytes-len(suffix)) + suffix
	}
	stem = truncateUTF8Bytes(stem, stemBytes)
	return stem + suffix + ext
}
