package conpty

import (
	"fmt"
	"os"
	"strings"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Spawn spawns a new process attached to the pseudo terminal
func Spawn(conpty *ConPty, argv0 string, argv []string, attr *syscall.ProcAttr) (pid int, handle uintptr, err error) {
	startupInfo := &startupInfoEx{}
	var attrListSize uint64
	startupInfo.startupInfo.Cb = uint32(unsafe.Sizeof(startupInfo))

	err = initializeProcThreadAttributeList(0, 1, &attrListSize)
	if err != nil {
		return 0, 0, fmt.Errorf("could not retrieve list size: %v", err)
	}

	attributeListBuffer := make([]byte, attrListSize)
	startupInfo.lpAttributeList = windows.Handle(unsafe.Pointer(&attributeListBuffer[0]))

	err = initializeProcThreadAttributeList(uintptr(startupInfo.lpAttributeList), 1, &attrListSize)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to initialize proc thread attributes for conpty: %v", err)
	}

	err = updateProcThreadAttributeList(
		startupInfo.lpAttributeList,
		procThreadAttributePseudoconsole,
		conpty.hpCon,
		unsafe.Sizeof(conpty.hpCon))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to update proc thread attributes attributes for conpty usage: %v", err)
	}

	if attr == nil {
		attr = &syscall.ProcAttr{}
	}

	if len(attr.Dir) != 0 {
		// StartProcess assumes that argv0 is relative to attr.Dir,
		// because it implies Chdir(attr.Dir) before executing argv0.
		// Windows CreateProcess assumes the opposite: it looks for
		// argv0 relative to the current directory, and, only once the new
		// process is started, it does Chdir(attr.Dir). We are adjusting
		// for that difference here by making argv0 absolute.
		var err error
		argv0, err = joinExeDirAndFName(attr.Dir, argv0)
		if err != nil {
			return 0, 0, err
		}
	}
	argv0p, err := windows.UTF16PtrFromString(argv0)
	if err != nil {
		return 0, 0, err
	}

	// Windows CreateProcess takes the command line as a single string:
	// use attr.CmdLine if set, else build the command line by escaping
	// and joining each argument with spaces
	cmdline := makeCmdLine(argv)

	var argvp *uint16
	if len(cmdline) != 0 {
		argvp, err = windows.UTF16PtrFromString(cmdline)
		if err != nil {
			return 0, 0, fmt.Errorf("utf ptr from string: %w", err)
		}
	}

	var dirp *uint16
	if len(attr.Dir) != 0 {
		dirp, err = windows.UTF16PtrFromString(attr.Dir)
		if err != nil {
			return 0, 0, fmt.Errorf("utf ptr from string: %w", err)
		}
	}

	startupInfo.startupInfo.Flags = windows.STARTF_USESTDHANDLES

	pi := new(windows.ProcessInformation)

	flags := uint32(windows.CREATE_UNICODE_ENVIRONMENT) | extendedStartupinfoPresent

	var zeroSec windows.SecurityAttributes
	pSec := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(zeroSec)), InheritHandle: 1}
	tSec := &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(zeroSec)), InheritHandle: 1}

	// c.startupInfo.startupInfo.Cb = uint32(unsafe.Sizeof(c.startupInfo))
	err = windows.CreateProcess(
		argv0p,
		argvp,
		pSec, // process handle not inheritable
		tSec, // thread handles not inheritable,
		false,
		flags,
		createEnvBlock(addCriticalEnv(dedupEnvCase(true, attr.Env))),
		dirp, // use current directory later: dirp,
		&startupInfo.startupInfo,
		pi)

	if err != nil {
		return 0, 0, fmt.Errorf("create process: %w", err)
	}
	defer windows.CloseHandle(windows.Handle(pi.Thread))

	return int(pi.ProcessId), uintptr(pi.Process), nil
}

// makeCmdLine builds a command line out of args by escaping "special"
// characters and joining the arguments with spaces.
func makeCmdLine(args []string) string {
	var s string
	for _, v := range args {
		if s != "" {
			s += " "
		}
		s += windows.EscapeArg(v)
	}
	return s
}

func isSlash(c uint8) bool {
	return c == '\\' || c == '/'
}

func normalizeDir(dir string) (name string, err error) {
	ndir, err := syscall.FullPath(dir)
	if err != nil {
		return "", err
	}
	if len(ndir) > 2 && isSlash(ndir[0]) && isSlash(ndir[1]) {
		// dir cannot have \\server\share\path form
		return "", syscall.EINVAL
	}
	return ndir, nil
}

func volToUpper(ch int) int {
	if 'a' <= ch && ch <= 'z' {
		ch += 'A' - 'a'
	}
	return ch
}

func joinExeDirAndFName(dir, p string) (name string, err error) {
	if len(p) == 0 {
		return "", syscall.EINVAL
	}
	if len(p) > 2 && isSlash(p[0]) && isSlash(p[1]) {
		// \\server\share\path form
		return p, nil
	}
	if len(p) > 1 && p[1] == ':' {
		// has drive letter
		if len(p) == 2 {
			return "", syscall.EINVAL
		}
		if isSlash(p[2]) {
			return p, nil
		} else {
			d, err := normalizeDir(dir)
			if err != nil {
				return "", err
			}
			if volToUpper(int(p[0])) == volToUpper(int(d[0])) {
				return syscall.FullPath(d + "\\" + p[2:])
			} else {
				return syscall.FullPath(p)
			}
		}
	} else {
		// no drive letter
		d, err := normalizeDir(dir)
		if err != nil {
			return "", err
		}
		if isSlash(p[0]) {
			return windows.FullPath(d[:2] + p)
		} else {
			return windows.FullPath(d + "\\" + p)
		}
	}
}

// createEnvBlock converts an array of environment strings into
// the representation required by CreateProcess: a sequence of NUL
// terminated strings followed by a nil.
// Last bytes are two UCS-2 NULs, or four NUL bytes.
func createEnvBlock(envv []string) *uint16 {
	if len(envv) == 0 {
		return &utf16.Encode([]rune("\x00\x00"))[0]
	}
	length := 0
	for _, s := range envv {
		length += len(s) + 1
	}
	length += 1

	b := make([]byte, length)
	i := 0
	for _, s := range envv {
		l := len(s)
		copy(b[i:i+l], []byte(s))
		copy(b[i+l:i+l+1], []byte{0})
		i = i + l + 1
	}
	copy(b[i:i+1], []byte{0})

	return &utf16.Encode([]rune(string(b)))[0]
}

// dedupEnvCase is dedupEnv with a case option for testing.
// If caseInsensitive is true, the case of keys is ignored.
func dedupEnvCase(caseInsensitive bool, env []string) []string {
	out := make([]string, 0, len(env))
	saw := make(map[string]int, len(env)) // key => index into out
	for _, kv := range env {
		eq := strings.Index(kv, "=")
		if eq < 0 {
			out = append(out, kv)
			continue
		}
		k := kv[:eq]
		if caseInsensitive {
			k = strings.ToLower(k)
		}
		if dupIdx, isDup := saw[k]; isDup {
			out[dupIdx] = kv
			continue
		}
		saw[k] = len(out)
		out = append(out, kv)
	}
	return out
}

// addCriticalEnv adds any critical environment variables that are required
// (or at least almost always required) on the operating system.
// Currently this is only used for Windows.
func addCriticalEnv(env []string) []string {
	for _, kv := range env {
		eq := strings.Index(kv, "=")
		if eq < 0 {
			continue
		}
		k := kv[:eq]
		if strings.EqualFold(k, "SYSTEMROOT") {
			// We already have it.
			return env
		}
	}
	return append(env, "SYSTEMROOT="+os.Getenv("SYSTEMROOT"))
}
