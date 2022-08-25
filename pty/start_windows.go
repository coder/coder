//go:build windows
// +build windows

package pty

import (
	"os"
	"os/exec"
	"strings"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/xerrors"
)

// Allocates a PTY and starts the specified command attached to it.
// See: https://docs.microsoft.com/en-us/windows/console/creating-a-pseudoconsole-session#creating-the-hosted-process
func startPty(cmd *exec.Cmd) (PTY, Process, error) {
	fullPath, err := exec.LookPath(cmd.Path)
	if err != nil {
		return nil, nil, err
	}
	pathPtr, err := windows.UTF16PtrFromString(fullPath)
	if err != nil {
		return nil, nil, err
	}
	argsPtr, err := windows.UTF16PtrFromString(windows.ComposeCommandLine(cmd.Args))
	if err != nil {
		return nil, nil, err
	}
	if cmd.Dir == "" {
		cmd.Dir, err = os.Getwd()
		if err != nil {
			return nil, nil, err
		}
	}
	dirPtr, err := windows.UTF16PtrFromString(cmd.Dir)
	if err != nil {
		return nil, nil, err
	}
	pty, err := newPty()
	if err != nil {
		return nil, nil, err
	}
	winPty := pty.(*ptyWindows)

	attrs, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		return nil, nil, err
	}
	// Taken from: https://github.com/microsoft/hcsshim/blob/2314362e977aa03b3ed245a4beb12d00422af0e2/internal/winapi/process.go#L6
	err = attrs.Update(0x20016, unsafe.Pointer(winPty.console), unsafe.Sizeof(winPty.console))
	if err != nil {
		return nil, nil, err
	}

	startupInfo := &windows.StartupInfoEx{}
	startupInfo.ProcThreadAttributeList = attrs.List()
	startupInfo.StartupInfo.Flags = windows.STARTF_USESTDHANDLES
	startupInfo.StartupInfo.Cb = uint32(unsafe.Sizeof(*startupInfo))
	var processInfo windows.ProcessInformation
	err = windows.CreateProcess(
		pathPtr,
		argsPtr,
		nil,
		nil,
		false,
		// https://docs.microsoft.com/en-us/windows/win32/procthread/process-creation-flags#create_unicode_environment
		windows.CREATE_UNICODE_ENVIRONMENT|windows.EXTENDED_STARTUPINFO_PRESENT,
		createEnvBlock(addCriticalEnv(dedupEnvCase(true, cmd.Env))),
		dirPtr,
		&startupInfo.StartupInfo,
		&processInfo,
	)
	if err != nil {
		return nil, nil, err
	}
	defer windows.CloseHandle(processInfo.Thread)
	defer windows.CloseHandle(processInfo.Process)

	process, err := os.FindProcess(int(processInfo.ProcessId))
	if err != nil {
		return nil, nil, xerrors.Errorf("find process %d: %w", processInfo.ProcessId, err)
	}
	wp := &windowsProcess{
		cmdDone: make(chan any),
		proc:    process,
	}
	go wp.waitInternal()
	return pty, wp, nil
}

// Taken from: https://github.com/microsoft/hcsshim/blob/7fbdca16f91de8792371ba22b7305bf4ca84170a/internal/exec/exec.go#L476
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
