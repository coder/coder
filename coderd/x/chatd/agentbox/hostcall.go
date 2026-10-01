package agentbox

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"sync/atomic"

	experimentalsys "github.com/tetratelabs/wazero/experimental/sys"
	"github.com/tetratelabs/wazero/sys"
)

// guestHostCallDir is the guest mount serving host calls. The guest writes
// a request to the single file in it and reads the response back.
const guestHostCallDir = "/mcp"

// hostCallFileName is the file under guestHostCallDir.
const hostCallFileName = "call"

// MaxHostCallRequestBytes caps one guest request. A larger request fails
// the write with EIO and the read returns an error envelope.
const MaxHostCallRequestBytes = 1 << 20

// HostCallFunc serves one guest request: request is the bytes the guest
// wrote and the returned bytes are what it reads back. An error is
// delivered to the guest as an error envelope instead of aborting the run.
// The function runs while Box.Run holds the box lock, so it must not call
// Box methods. ctx ends when the run ends for any reason: timeout, box or
// engine close, the caller's context, or the guest exiting.
type HostCallFunc func(ctx context.Context, request []byte) ([]byte, error)

// HostCallEnvelope is the JSON the guest reads for every host call.
type HostCallEnvelope struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
	Code   string          `json:"code,omitempty"`
}

// Host call envelope codes set by the channel itself.
const (
	HostCallCodeRequestTooLarge = "request_too_large"
	HostCallCodeCanceled        = "canceled"
	HostCallCodeInternal        = "internal"
)

// HostCallError marshals an error envelope.
func HostCallError(code, msg string) []byte {
	out, err := json.Marshal(HostCallEnvelope{Error: msg, Code: code})
	if err != nil {
		return []byte(`{"ok":false,"code":"internal","error":"encode envelope"}`)
	}
	return out
}

// HostCallResult marshals a success envelope around an already encoded
// result.
func HostCallResult(result json.RawMessage) []byte {
	out, err := json.Marshal(HostCallEnvelope{OK: true, Result: result})
	if err != nil {
		return HostCallError(HostCallCodeInternal, "encode envelope: "+err.Error())
	}
	return out
}

// hostCallFS is a read-write mount with one file. Each open yields a fresh
// request/response exchange. One call runs at a time: quickjs is single
// threaded, so the slot only matters when a canceled call is still
// finishing in its goroutine.
type hostCallFS struct {
	experimentalsys.UnimplementedFS
	ctx  context.Context
	call HostCallFunc
	slot chan struct{}
	// requestTooLarge is set when a guest write passed the request cap.
	requestTooLarge atomic.Bool
	// canceled is set once a canceled envelope was served. A guest that
	// exits on it before the module is flagged closed was still
	// interrupted.
	canceled atomic.Bool
}

func newHostCallFS(ctx context.Context, call HostCallFunc) *hostCallFS {
	return &hostCallFS{ctx: ctx, call: call, slot: make(chan struct{}, 1)}
}

func (f *hostCallFS) OpenFile(p string, _ experimentalsys.Oflag, _ fs.FileMode) (experimentalsys.File, experimentalsys.Errno) {
	switch p {
	case ".", "", "/":
		return &hostCallDir{}, 0
	case hostCallFileName:
		return &hostCallFile{fs: f}, 0
	default:
		return nil, experimentalsys.ENOENT
	}
}

func (*hostCallFS) Stat(p string) (sys.Stat_t, experimentalsys.Errno) {
	switch p {
	case ".", "", "/":
		return dirStat(), 0
	case hostCallFileName:
		return fileStat(), 0
	default:
		return sys.Stat_t{}, experimentalsys.ENOENT
	}
}

func (f *hostCallFS) Lstat(p string) (sys.Stat_t, experimentalsys.Errno) {
	return f.Stat(p)
}

func dirStat() sys.Stat_t {
	return sys.Stat_t{Mode: fs.ModeDir | 0o500, Nlink: 1}
}

func fileStat() sys.Stat_t {
	return sys.Stat_t{Mode: 0o600, Nlink: 1}
}

// hostCallDir is the mount root. It must open and stat as a directory;
// any errno other than ENOENT from the root open is fatal to the mount.
type hostCallDir struct {
	experimentalsys.UnimplementedFile
	listed bool
}

func (hostCallDir) IsDir() (bool, experimentalsys.Errno)      { return true, 0 }
func (hostCallDir) Stat() (sys.Stat_t, experimentalsys.Errno) { return dirStat(), 0 }
func (hostCallDir) Read([]byte) (int, experimentalsys.Errno)  { return 0, experimentalsys.EISDIR }
func (hostCallDir) Write([]byte) (int, experimentalsys.Errno) { return 0, experimentalsys.EISDIR }
func (d *hostCallDir) Readdir(n int) ([]experimentalsys.Dirent, experimentalsys.Errno) {
	if n == 0 || d.listed {
		return nil, 0
	}
	d.listed = true
	return []experimentalsys.Dirent{{Name: hostCallFileName, Type: 0}}, 0
}
func (hostCallDir) Close() experimentalsys.Errno { return 0 }

// hostCallFile is one exchange. Writes buffer the request; the first read
// serves the call and later reads continue through the response.
type hostCallFile struct {
	experimentalsys.UnimplementedFile
	fs       *hostCallFS
	request  []byte
	response []byte
	// served is set once the response is fixed, by the first read or by a
	// failed write.
	served bool
	offset int64
}

func (*hostCallFile) Stat() (sys.Stat_t, experimentalsys.Errno) { return fileStat(), 0 }
func (*hostCallFile) IsDir() (bool, experimentalsys.Errno)      { return false, 0 }
func (*hostCallFile) Close() experimentalsys.Errno              { return 0 }
func (*hostCallFile) Sync() experimentalsys.Errno               { return 0 }

func (f *hostCallFile) Write(buf []byte) (int, experimentalsys.Errno) {
	if f.served {
		return 0, experimentalsys.EIO
	}
	if len(f.request)+len(buf) > MaxHostCallRequestBytes {
		f.fs.requestTooLarge.Store(true)
		f.fail(HostCallCodeRequestTooLarge, "request exceeds the host call request limit")
		return 0, experimentalsys.EIO
	}
	f.request = append(f.request, buf...)
	return len(buf), 0
}

func (f *hostCallFile) fail(code, msg string) {
	f.response = HostCallError(code, msg)
	f.served = true
}

// serve runs the host call once and fixes the response. The call runs in
// its own goroutine so a callee that ignores ctx cannot block the guest
// after the run is canceled.
func (f *hostCallFile) serve() {
	if f.served {
		return
	}
	f.served = true
	ctx := f.fs.ctx
	if ctx.Err() != nil {
		f.cancel()
		return
	}
	type outcome struct {
		out []byte
		err error
	}
	select {
	case f.fs.slot <- struct{}{}:
	case <-ctx.Done():
		f.cancel()
		return
	}
	done := make(chan outcome, 1)
	request := f.request
	go func() {
		defer func() { <-f.fs.slot }()
		out, err := f.fs.call(ctx, request)
		done <- outcome{out: out, err: err}
	}()
	select {
	case res := <-done:
		switch {
		case res.err != nil && ctx.Err() != nil && errors.Is(res.err, context.Canceled):
			f.cancel()
		case res.err != nil:
			f.response = HostCallError(HostCallCodeInternal, res.err.Error())
		case len(res.out) == 0:
			f.response = HostCallError(HostCallCodeInternal, "empty host response")
		default:
			f.response = res.out
		}
	case <-ctx.Done():
		f.cancel()
	}
}

func (f *hostCallFile) cancel() {
	f.fs.canceled.Store(true)
	f.response = HostCallError(HostCallCodeCanceled, "run canceled")
}

func (f *hostCallFile) Read(buf []byte) (int, experimentalsys.Errno) {
	f.serve()
	if f.offset >= int64(len(f.response)) {
		return 0, 0
	}
	n := copy(buf, f.response[f.offset:])
	f.offset += int64(n)
	return n, 0
}

func (f *hostCallFile) Pread(buf []byte, off int64) (int, experimentalsys.Errno) {
	if off < 0 {
		return 0, experimentalsys.EINVAL
	}
	f.serve()
	if off >= int64(len(f.response)) {
		return 0, 0
	}
	return copy(buf, f.response[off:]), 0
}

func (f *hostCallFile) Seek(offset int64, whence int) (int64, experimentalsys.Errno) {
	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = f.offset
	case io.SeekEnd:
		f.serve()
		base = int64(len(f.response))
	default:
		return 0, experimentalsys.EINVAL
	}
	next := base + offset
	if next < 0 {
		return 0, experimentalsys.EINVAL
	}
	f.offset = next
	return next, 0
}
