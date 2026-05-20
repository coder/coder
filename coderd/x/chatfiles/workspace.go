package chatfiles

import (
	"path"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
)

const (
	// MaxWorkspaceUploadFileNameBytes caps the sanitized basename of a
	// workspace upload. It mirrors MaxStoredFileNameBytes so the chat
	// composer and workspace flows agree on the longest allowed name.
	MaxWorkspaceUploadFileNameBytes = 255

	// WorkspaceChatsDir is the directory under the agent's home directory
	// that holds per-chat coder artifacts. Sits under the existing
	// `~/.coder/` namespace so chat data lives next to other coder
	// configuration rather than at the top of the home directory.
	WorkspaceChatsDir = ".coder/chats"

	// WorkspaceUploadFilesSubdir is the subdirectory of a chat's
	// workspace folder that holds uploaded files.
	WorkspaceUploadFilesSubdir = "files"
)

// ErrWorkspaceUploadNameRequired indicates that a workspace upload name
// is empty after sanitization.
var ErrWorkspaceUploadNameRequired = xerrors.New("workspace upload file name is required")

// unsafeWorkspaceUploadChars matches characters that must be replaced
// in the base name (control chars and ASCII space). Path separators
// are handled by path.Base; all other Unicode letters and symbols
// pass through.
var unsafeWorkspaceUploadChars = regexp.MustCompile(`[[:cntrl:] ]+`)

// SanitizeWorkspaceUploadName returns a basename safe to use as the
// final path component of a workspace upload. It normalizes Windows
// path separators, strips path components, collapses runs of control
// characters or whitespace into a single underscore, trims leading
// dots so the file cannot masquerade as a dotfile, and truncates the
// result to MaxWorkspaceUploadFileNameBytes without splitting UTF-8
// runes.
//
// Returns ErrWorkspaceUploadNameRequired when the sanitized name is
// empty so callers cannot accidentally write to the parent directory.
func SanitizeWorkspaceUploadName(name string) (string, error) {
	name = strings.ReplaceAll(strings.TrimSpace(name), `\`, "/")
	name = path.Base(name)
	if name == "." || name == "/" {
		return "", ErrWorkspaceUploadNameRequired
	}
	name = unsafeWorkspaceUploadChars.ReplaceAllString(name, "_")
	name = strings.TrimLeft(name, ".")
	name = truncateUTF8Bytes(name, MaxWorkspaceUploadFileNameBytes)
	if name == "" || name == "_" {
		return "", ErrWorkspaceUploadNameRequired
	}
	return name, nil
}

// WorkspaceChatDir returns the per-chat directory under the agent's
// home directory: $HOME/.coder/chats/<chat-id>. chatID is expected to
// be the full chat UUID; the caller is responsible for validating it.
func WorkspaceChatDir(homeDir, chatID string) string {
	if homeDir == "" {
		homeDir = "~"
	}
	return path.Join(homeDir, WorkspaceChatsDir, chatID)
}

// WorkspaceUploadDir returns the per-chat directory under the agent's
// home directory where uploads should be written: $HOME/.coder/chats/<chat-id>/files.
// chatID is expected to be the full chat UUID; the caller is responsible
// for validating it.
func WorkspaceUploadDir(homeDir, chatID string) string {
	return path.Join(WorkspaceChatDir(homeDir, chatID), WorkspaceUploadFilesSubdir)
}

// AddCollisionSuffix returns a candidate filename with a `_<n>` suffix
// inserted before the extension, e.g. "foo.zip" with n=2 becomes
// "foo_2.zip". When the filename has no extension the suffix is
// appended. For extension-only names like ".env" the suffix is
// appended to the whole name (".env_2"). n <= 1 returns the name
// unchanged.
func AddCollisionSuffix(name string, n int) string {
	if n <= 1 {
		return name
	}
	ext := path.Ext(name)
	stem := strings.TrimSuffix(name, ext)
	if stem == "" {
		stem = name
		ext = ""
	}
	return stem + "_" + strconv.Itoa(n) + ext
}
