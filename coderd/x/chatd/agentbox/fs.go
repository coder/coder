package agentbox

import (
	"io"
	"io/fs"
	"path"
	"sync"
	"sync/atomic"

	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/sys"
)

// entryCost is charged against the disk quota for every file or
// directory created so an empty-entry loop is bounded like data writes.
const entryCost = 4096

// maxOpenFiles bounds the host file descriptors one run holds across
// both guest mounts.
const maxOpenFiles = 256

// errQuotaExceeded is what the guest sees when a write would exceed the
// disk quota. wazero's errno set has no ENOSPC.
const errQuotaExceeded = experimentalsys.EIO

// errTooManyOpenFiles is what the guest sees at maxOpenFiles. wazero's
// errno set has no EMFILE.
const errTooManyOpenFiles = experimentalsys.EIO

// quota is the remaining disk budget of a box, shared by the guest mount
// and host-side writes. The charged total is the apparent size of every
// regular file plus entryCost per created entry, so refunding a file's
// reported size on removal or truncation is exact. Bytes of a file
// replaced by rename are never refunded.
type quota struct {
	remaining atomic.Int64
}

func newQuota(limit int64) *quota {
	q := &quota{}
	q.remaining.Store(limit)
	return q
}

// charge reserves n bytes and reports whether the budget allowed it.
func (q *quota) charge(n int64) bool {
	if n <= 0 {
		return true
	}
	if q.remaining.Add(-n) < 0 {
		q.remaining.Add(n)
		return false
	}
	return true
}

func (q *quota) refund(n int64) {
	if n > 0 {
		q.remaining.Add(n)
	}
}

// isMountRoot reports whether a mount-relative path names the mount
// itself.
func isMountRoot(p string) bool {
	return path.Clean(p) == "."
}

// boxFS is the guest-writable /box mount of one run. It relies on the
// invariant that no symlink exists under the box root: the guest cannot
// create one (Symlink and Link are denied) and host writes go through
// os.Root, so the host-side path joining performed by the wrapped DirFS
// cannot escape.
//
// Removing a file that is still open keeps its bytes on disk, so boxFS
// counts open handles per inode and refunds an unlinked file's bytes when
// its last handle closes. Where the platform reports no inode number,
// unlinking refunds only the entry cost.
type boxFS struct {
	experimentalsys.FS
	quota *quota

	// quotaHit records that a guest call failed on the quota.
	quotaHit atomic.Bool

	mu       sync.Mutex
	open     map[sys.Inode]int
	orphaned map[sys.Inode]bool
}

func (f *boxFS) charge(n int64) bool {
	if f.quota.charge(n) {
		return true
	}
	f.quotaHit.Store(true)
	return false
}

func newBoxFS(inner experimentalsys.FS, q *quota) *boxFS {
	return &boxFS{
		FS:       inner,
		quota:    q,
		open:     make(map[sys.Inode]int),
		orphaned: make(map[sys.Inode]bool),
	}
}

func (f *boxFS) OpenFile(p string, flag experimentalsys.Oflag, perm fs.FileMode) (experimentalsys.File, experimentalsys.Errno) {
	created := false
	var truncated int64
	if flag&(experimentalsys.O_CREAT|experimentalsys.O_TRUNC) != 0 {
		st, errno := f.Lstat(p)
		switch errno {
		case 0:
			if flag&experimentalsys.O_TRUNC != 0 && st.Mode.IsRegular() {
				truncated = st.Size
			}
		case experimentalsys.ENOENT:
			created = flag&experimentalsys.O_CREAT != 0
		default:
			return nil, errno
		}
	}
	if created && !f.charge(entryCost) {
		return nil, errQuotaExceeded
	}
	file, errno := f.FS.OpenFile(p, flag, perm)
	if errno != 0 {
		if created {
			f.quota.refund(entryCost)
		}
		return nil, errno
	}
	f.quota.refund(truncated)

	st, errno := file.Stat()
	if errno != 0 {
		_ = file.Close()
		return nil, errno
	}
	qf := &quotaFile{File: file, fs: f}
	if st.Mode.IsRegular() && st.Ino != 0 {
		qf.ino = st.Ino
		f.mu.Lock()
		f.open[qf.ino]++
		f.mu.Unlock()
	}
	return qf, 0
}

// release drops one open handle of ino and refunds the bytes of an
// unlinked inode when its last handle closes. size is the inode's size
// at close.
func (f *boxFS) release(ino sys.Inode, size int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.open[ino]--
	if f.open[ino] > 0 {
		return
	}
	delete(f.open, ino)
	if f.orphaned[ino] {
		delete(f.orphaned, ino)
		f.quota.refund(size)
	}
}

func (f *boxFS) Mkdir(p string, perm fs.FileMode) experimentalsys.Errno {
	if !f.charge(entryCost) {
		return errQuotaExceeded
	}
	errno := f.FS.Mkdir(p, perm)
	if errno != 0 {
		f.quota.refund(entryCost)
	}
	return errno
}

func (f *boxFS) Rmdir(p string) experimentalsys.Errno {
	if isMountRoot(p) {
		return experimentalsys.EPERM
	}
	errno := f.FS.Rmdir(p)
	if errno == 0 {
		f.quota.refund(entryCost)
	}
	return errno
}

func (f *boxFS) Rename(from, to string) experimentalsys.Errno {
	if isMountRoot(from) || isMountRoot(to) {
		return experimentalsys.EPERM
	}
	return f.FS.Rename(from, to)
}

func (f *boxFS) Unlink(p string) experimentalsys.Errno {
	if isMountRoot(p) {
		return experimentalsys.EPERM
	}
	var (
		regular bool
		size    int64
		ino     sys.Inode
	)
	if st, errno := f.Lstat(p); errno == 0 && st.Mode.IsRegular() {
		regular, size, ino = true, st.Size, st.Ino
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	errno := f.FS.Unlink(p)
	if errno != 0 {
		return errno
	}
	switch {
	case !regular || ino == 0:
		f.quota.refund(entryCost)
	case f.open[ino] > 0:
		f.orphaned[ino] = true
		f.quota.refund(entryCost)
	default:
		f.quota.refund(size + entryCost)
	}
	return 0
}

func (*boxFS) Link(_, _ string) experimentalsys.Errno {
	return experimentalsys.EPERM
}

func (*boxFS) Symlink(_, _ string) experimentalsys.Errno {
	return experimentalsys.EPERM
}

// quotaFile charges apparent-size growth against the box quota. Guest
// calls on one run are serialized, so the size read before a write is
// current.
type quotaFile struct {
	experimentalsys.File
	fs *boxFS
	// ino is zero when the handle is not tracked for orphan refunds.
	ino    sys.Inode
	closed bool
}

func (f *quotaFile) size() (int64, experimentalsys.Errno) {
	st, errno := f.Stat()
	if errno != 0 {
		return 0, errno
	}
	return st.Size, 0
}

// chargedWrite charges the growth a write of n bytes at off would cause, runs
// do, and refunds the part of the charge the write did not use.
func (f *quotaFile) chargedWrite(off int64, n int, do func() (int, experimentalsys.Errno)) (int, experimentalsys.Errno) {
	size, errno := f.size()
	if errno != 0 {
		return 0, errno
	}
	if f.IsAppend() {
		off = size
	}
	charged := max(0, off+int64(n)-size)
	if !f.fs.charge(charged) {
		return 0, errQuotaExceeded
	}
	written, errno := do()
	used := max(0, off+int64(written)-size)
	f.fs.quota.refund(charged - used)
	return written, errno
}

func (f *quotaFile) Write(buf []byte) (int, experimentalsys.Errno) {
	off, errno := f.Seek(0, io.SeekCurrent)
	if errno != 0 {
		return 0, errno
	}
	return f.chargedWrite(off, len(buf), func() (int, experimentalsys.Errno) {
		return f.File.Write(buf)
	})
}

func (f *quotaFile) Pwrite(buf []byte, off int64) (int, experimentalsys.Errno) {
	return f.chargedWrite(off, len(buf), func() (int, experimentalsys.Errno) {
		return f.File.Pwrite(buf, off)
	})
}

func (f *quotaFile) Truncate(size int64) experimentalsys.Errno {
	current, errno := f.size()
	if errno != 0 {
		return errno
	}
	growth := size - current
	if !f.fs.charge(growth) {
		return errQuotaExceeded
	}
	errno = f.File.Truncate(size)
	if errno != 0 {
		f.fs.quota.refund(growth)
		return errno
	}
	f.fs.quota.refund(-growth)
	return 0
}

func (f *quotaFile) Close() experimentalsys.Errno {
	if f.closed {
		return 0
	}
	f.closed = true
	if f.ino != 0 {
		// A failed stat leaves an orphan's bytes charged.
		size, _ := f.size()
		f.fs.release(f.ino, size)
	}
	return f.File.Close()
}

// openLimit counts host file descriptors held by one run.
type openLimit struct {
	open atomic.Int64
	max  int64
	// hit records that an open failed on the limit.
	hit atomic.Bool
}

func (l *openLimit) acquire() bool {
	if l.open.Add(1) > l.max {
		l.open.Add(-1)
		l.hit.Store(true)
		return false
	}
	return true
}

func (l *openLimit) release() {
	l.open.Add(-1)
}

// limitedFS fails opens once its run holds limit.max descriptors.
type limitedFS struct {
	experimentalsys.FS
	limit *openLimit
}

func (f limitedFS) OpenFile(p string, flag experimentalsys.Oflag, perm fs.FileMode) (experimentalsys.File, experimentalsys.Errno) {
	if !f.limit.acquire() {
		return nil, errTooManyOpenFiles
	}
	file, errno := f.FS.OpenFile(p, flag, perm)
	if errno != 0 {
		f.limit.release()
		return nil, errno
	}
	return &limitedFile{File: file, limit: f.limit}, 0
}

type limitedFile struct {
	experimentalsys.File
	limit    *openLimit
	released bool
}

func (f *limitedFile) Close() experimentalsys.Errno {
	if f.released {
		return 0
	}
	f.released = true
	defer f.limit.release()
	return f.File.Close()
}
