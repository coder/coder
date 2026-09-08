// Package desktopruntime exposes the Linux desktop runtime archive that is
// embedded into the workspace agent binary. The archive contains a static
// Xvnc server, xkbcomp, and a trimmed keymap set, built by
// scripts/desktopruntime at Coder build time. Inside the workspace the agent
// unpacks the archive and execs bin/Xvnc directly; it has no dependency on
// Docker, the network, or host libraries.
//
// The archive is only compiled in for linux/amd64 and linux/arm64 builds
// that set the desktop_runtime build tag. Every other build sees an empty
// archive and Available reports false.
package desktopruntime

import (
	"crypto/sha256"
	"encoding/hex"
	"sync"
)

// ArchiveName is the file name of the embedded archive for the current
// GOOS/GOARCH, or empty when no archive is compiled in.
const ArchiveName = archiveName

// Available reports whether a desktop runtime archive is compiled into this
// binary.
func Available() bool {
	return len(archive) > 0
}

// Archive returns the zstd-compressed tar archive of the runtime root. The
// returned slice must not be modified. It is nil when Available is false.
func Archive() []byte {
	return archive
}

var (
	sumOnce sync.Once
	sum     string
)

// SHA256 returns the hex-encoded SHA-256 of the embedded archive. It is used
// to key the unpacked cache directory so that upgrading the agent binary
// never reuses a stale runtime. It returns an empty string when Available is
// false.
func SHA256() string {
	sumOnce.Do(func() {
		if len(archive) == 0 {
			return
		}
		h := sha256.Sum256(archive)
		sum = hex.EncodeToString(h[:])
	})
	return sum
}
