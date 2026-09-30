package agentbox

import (
	"io/fs"
	"sync/atomic"

	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
)

// entryCost is charged against the disk quota for every file or
// directory created so an empty-entry loop is bounded like data writes.
const entryCost = 4096

// errQuotaExceeded is what the guest sees when a write would exceed the
// disk quota. wazero's errno set has no ENOSPC.
const errQuotaExceeded = experimentalsys.EIO

// quota is the remaining disk budget of a box, shared by the guest mount
// and host-side writes. Overwrites are charged as new bytes; only removal,
// truncation, and shrink refund.
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

// boxFS is the guest-writable /box mount. It relies on the invariant that
// no symlink exists under the box root: the guest cannot create one
// (Symlink and Link are denied) and host writes go through os.Root, so the
// host-side path joining performed by the wrapped DirFS cannot escape.
type boxFS struct {
	experimentalsys.FS
	quota *quota
}

func newBoxFS(inner experimentalsys.FS, q *quota) boxFS {
	return boxFS{FS: inner, quota: q}
}

func (f boxFS) OpenFile(path string, flag experimentalsys.Oflag, perm fs.FileMode) (experimentalsys.File, experimentalsys.Errno) {
	created := false
	var truncated int64
	if flag&(experimentalsys.O_CREAT|experimentalsys.O_TRUNC) != 0 {
		st, errno := f.Lstat(path)
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
	if created && !f.quota.charge(entryCost) {
		return nil, errQuotaExceeded
	}
	file, errno := f.FS.OpenFile(path, flag, perm)
	if errno != 0 {
		if created {
			f.quota.refund(entryCost)
		}
		return nil, errno
	}
	f.quota.refund(truncated)
	return &quotaFile{File: file, quota: f.quota}, 0
}

func (f boxFS) Mkdir(path string, perm fs.FileMode) experimentalsys.Errno {
	if !f.quota.charge(entryCost) {
		return errQuotaExceeded
	}
	errno := f.FS.Mkdir(path, perm)
	if errno != 0 {
		f.quota.refund(entryCost)
	}
	return errno
}

func (f boxFS) Rmdir(path string) experimentalsys.Errno {
	errno := f.FS.Rmdir(path)
	if errno == 0 {
		f.quota.refund(entryCost)
	}
	return errno
}

func (f boxFS) Unlink(path string) experimentalsys.Errno {
	var size int64
	if st, errno := f.Lstat(path); errno == 0 && st.Mode.IsRegular() {
		size = st.Size
	}
	errno := f.FS.Unlink(path)
	if errno == 0 {
		f.quota.refund(size + entryCost)
	}
	return errno
}

func (boxFS) Link(_, _ string) experimentalsys.Errno {
	return experimentalsys.EPERM
}

func (boxFS) Symlink(_, _ string) experimentalsys.Errno {
	return experimentalsys.EPERM
}

// quotaFile charges writes and growth against the box quota.
type quotaFile struct {
	experimentalsys.File
	quota *quota
}

func (f *quotaFile) Write(buf []byte) (int, experimentalsys.Errno) {
	if !f.quota.charge(int64(len(buf))) {
		return 0, errQuotaExceeded
	}
	n, errno := f.File.Write(buf)
	f.quota.refund(int64(len(buf) - n))
	return n, errno
}

func (f *quotaFile) Pwrite(buf []byte, off int64) (int, experimentalsys.Errno) {
	if !f.quota.charge(int64(len(buf))) {
		return 0, errQuotaExceeded
	}
	n, errno := f.File.Pwrite(buf, off)
	f.quota.refund(int64(len(buf) - n))
	return n, errno
}

func (f *quotaFile) Truncate(size int64) experimentalsys.Errno {
	st, errno := f.Stat()
	if errno != 0 {
		return errno
	}
	growth := size - st.Size
	if !f.quota.charge(growth) {
		return errQuotaExceeded
	}
	errno = f.File.Truncate(size)
	if errno != 0 {
		f.quota.refund(growth)
		return errno
	}
	f.quota.refund(-growth)
	return 0
}
