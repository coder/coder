// Package desktopruntime unpacks the Linux desktop runtime embedded in the
// workspace agent binary: a static Xvnc server, xkbcomp, and a trimmed keymap
// set provided by the github.com/coder/portabledesktop/runtime module. Inside
// the workspace the agent unpacks the archive and execs bin/Xvnc directly; it
// has no dependency on Docker, the network, or host libraries.
//
// The runtime module only embeds archives for linux/amd64 and linux/arm64.
// Every other build sees an empty archive and Available reports false.
package desktopruntime

import (
	pdruntime "github.com/coder/portabledesktop/runtime"
)

// XvncPath is the path of the Xvnc executable relative to an unpacked
// runtime root.
const XvncPath = pdruntime.XvncPath

// XKBDir is the path of the keymap data relative to an unpacked runtime root.
const XKBDir = pdruntime.XKBDir

// ArchiveName is the file name of the embedded archive for the current
// GOOS/GOARCH, or empty when no archive is compiled in.
const ArchiveName = pdruntime.ArchiveName

// Available reports whether a desktop runtime archive is compiled into this
// binary.
func Available() bool {
	return pdruntime.Available()
}

// Archive returns the zstd-compressed tar archive of the runtime root. The
// returned slice must not be modified. It is nil when Available is false.
func Archive() []byte {
	return pdruntime.Archive()
}

// SHA256 returns the hex-encoded SHA-256 of the embedded archive. It is used
// to key the unpacked cache directory so that upgrading the agent binary
// never reuses a stale runtime. It returns an empty string when Available is
// false.
func SHA256() string {
	return pdruntime.SHA256()
}
