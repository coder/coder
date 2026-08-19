package agentssh

import (
	"context"
	"path/filepath"
	"strings"
	"sync"

	"github.com/gliderlabs/ssh"
	"github.com/google/uuid"

	"cdr.dev/slog/v3"
)

// FileTransferProtocol identifies the protocol carrying a file-transfer
// session.
type FileTransferProtocol string

const (
	FileTransferProtocolSFTP  FileTransferProtocol = "sftp"
	FileTransferProtocolSCP   FileTransferProtocol = "scp"
	FileTransferProtocolRsync FileTransferProtocol = "rsync"
)

// FileTransferAction is the kind of file operation observed during a
// file-transfer session. A download is a transfer from the workspace to
// the client, an upload is a transfer from the client to the workspace,
// and bidirectional is a file opened for both at once, where either may
// have occurred.
type FileTransferAction string

const (
	FileTransferActionDownload      FileTransferAction = "download"
	FileTransferActionUpload        FileTransferAction = "upload"
	FileTransferActionBidirectional FileTransferAction = "bidirectional"
	FileTransferActionRemove        FileTransferAction = "remove"
	FileTransferActionRmdir         FileTransferAction = "rmdir"
	FileTransferActionRename        FileTransferAction = "rename"
	FileTransferActionSymlink       FileTransferAction = "symlink"
	// FileTransferActionSetattr records file attribute changes:
	// truncation, permissions, ownership, and timestamps.
	FileTransferActionSetattr FileTransferAction = "setattr"
	// FileTransferActionHardlink records creation of a hard link. Path
	// is the existing file, Target is the new link.
	FileTransferActionHardlink FileTransferAction = "hardlink"
)

// FileTransferOperation is a single observed file operation, reported to
// coderd for the connection log.
type FileTransferOperation struct {
	Protocol FileTransferProtocol
	Action   FileTransferAction
	Path     string
	// Target is the second path for operations that have one, such as
	// the destination of a rename or the target of a symlink.
	Target string
}

// FileTransferSession describes a session recognized as a file transfer.
type FileTransferSession struct {
	Protocol FileTransferProtocol
	// InitialOperation is the operation implied by the command line for
	// exec-based transfers (scp, rsync). Nil for SFTP, whose operations
	// are observed per-request, and for command lines whose direction
	// could not be determined.
	InitialOperation *FileTransferOperation
}

// classifyFileTransfer determines whether an SSH session is a file
// transfer. It recognizes the SFTP subsystem and exec sessions whose
// command is scp or rsync. nc is deliberately not classified: it is a
// generic networking tool with no reliable file semantics (it remains
// subject to BlockFileTransfer).
//
// Warning: like fileTransferBlocked, this is a "do not trespass" sign,
// not a security boundary. A user can rename binaries or wrap them in
// shell scripts to evade classification.
func classifyFileTransfer(session ssh.Session) (FileTransferSession, bool) {
	if session.Subsystem() == "sftp" {
		return FileTransferSession{Protocol: FileTransferProtocolSFTP}, true
	}
	if session.Subsystem() != "" {
		return FileTransferSession{}, false
	}

	cmd := session.Command()
	if len(cmd) == 0 {
		return FileTransferSession{}, false
	}

	// In case the binary is an absolute path, /usr/sbin/scp.
	switch filepath.Base(cmd[0]) {
	case "scp":
		fts := FileTransferSession{Protocol: FileTransferProtocolSCP}
		if op, ok := parseSCPCommand(cmd); ok {
			fts.InitialOperation = &op
		}
		return fts, true
	case "rsync":
		fts := FileTransferSession{Protocol: FileTransferProtocolRsync}
		if op, ok := parseRsyncCommand(cmd); ok {
			fts.InitialOperation = &op
		}
		return fts, true
	default:
		return FileTransferSession{}, false
	}
}

// parseSCPCommand extracts the direction and root path from a remote scp
// server invocation, e.g. "scp -r -t /tmp" or "scp -qf file". The remote
// side always runs with -t (sink mode: client uploads to the workspace)
// or -f (source mode: client downloads from the workspace), followed by
// the target path. The path is the requested root operand; recursive
// transfers copy files beneath it that are not individually visible at
// the command line.
func parseSCPCommand(cmd []string) (FileTransferOperation, bool) {
	var (
		action  FileTransferAction
		doneOpt bool
		path    string
	)
	for _, arg := range cmd[1:] {
		if !doneOpt && strings.HasPrefix(arg, "-") {
			if arg == "--" {
				doneOpt = true
				continue
			}
			// Flags may be combined, e.g. -qrt.
			if strings.ContainsRune(arg, 't') {
				action = FileTransferActionUpload
			}
			if strings.ContainsRune(arg, 'f') {
				action = FileTransferActionDownload
			}
			continue
		}
		// First non-flag operand is the path.
		path = arg
		break
	}
	if action == "" || path == "" {
		return FileTransferOperation{}, false
	}
	return FileTransferOperation{
		Protocol: FileTransferProtocolSCP,
		Action:   action,
		Path:     path,
	}, true
}

// parseRsyncCommand extracts the direction and root path from a remote
// rsync server invocation, e.g. "rsync --server --sender -vlogDtpre.iLsfxC . /tmp".
// The remote side runs with --server; --sender means the workspace sends
// (client downloads), otherwise the workspace receives (client uploads).
// The path is the last operand ("." is a placeholder preceding it).
func parseRsyncCommand(cmd []string) (FileTransferOperation, bool) {
	var (
		server  bool
		sender  bool
		doneOpt bool
		path    string
	)
	for _, arg := range cmd[1:] {
		if !doneOpt && strings.HasPrefix(arg, "-") {
			switch arg {
			case "--server":
				server = true
			case "--sender":
				sender = true
			case "--":
				doneOpt = true
			}
			continue
		}
		if arg == "." {
			continue
		}
		// The last operand is the path.
		path = arg
	}
	if !server || path == "" {
		return FileTransferOperation{}, false
	}
	action := FileTransferActionUpload
	if sender {
		action = FileTransferActionDownload
	}
	return FileTransferOperation{
		Protocol: FileTransferProtocolRsync,
		Action:   action,
		Path:     path,
	}, true
}

// magicTypeForFileTransfer returns whether a session with the given magic
// type should be reported as a file transfer when classified as one. Only
// plain SSH sessions are re-typed; IDE sessions (VS Code, JetBrains) keep
// their more specific type.
func magicTypeForFileTransfer(magicType MagicSessionType) bool {
	return magicType == MagicSessionTypeSSH || magicType == MagicSessionTypeUnknown
}

// maxFileTransferOpsPerSession bounds the number of file operations
// reported for a single session. Long-lived SFTP sessions (e.g. sshfs
// mounts) can touch a very large number of files; beyond the cap, new
// operations are dropped and a debug log records the loss.
const maxFileTransferOpsPerSession = 512

// fileTransferOpEmitter deduplicates and caps file operations for one
// session before handing them to the configured report function.
type fileTransferOpEmitter struct {
	logger slog.Logger
	id     uuid.UUID
	report func(id uuid.UUID, op FileTransferOperation)

	mu      sync.Mutex
	seen    map[string]struct{}
	dropped bool
}

func newFileTransferOpEmitter(logger slog.Logger, id uuid.UUID, report func(id uuid.UUID, op FileTransferOperation)) *fileTransferOpEmitter {
	return &fileTransferOpEmitter{
		logger: logger,
		id:     id,
		report: report,
		seen:   make(map[string]struct{}),
	}
}

// Emit reports one file operation. Identical operations within the same
// session are reported once; e.g. sshfs re-opens the same files
// constantly. Safe for concurrent use.
func (e *fileTransferOpEmitter) Emit(op FileTransferOperation) {
	key := string(op.Action) + "\x00" + op.Path + "\x00" + op.Target
	e.mu.Lock()
	if _, ok := e.seen[key]; ok {
		e.mu.Unlock()
		return
	}
	if len(e.seen) >= maxFileTransferOpsPerSession {
		if !e.dropped {
			e.dropped = true
			e.logger.Debug(context.Background(), "file transfer operation cap reached, dropping further operations",
				slog.F("cap", maxFileTransferOpsPerSession))
		}
		e.mu.Unlock()
		return
	}
	e.seen[key] = struct{}{}
	e.mu.Unlock()

	e.report(e.id, op)
}
