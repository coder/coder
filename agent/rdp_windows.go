package agent

import (
	"context"
	"fmt"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	wtsapi32 = windows.NewLazySystemDLL("wtsapi32.dll")

	// WTSQuerySessionInformationW
	procWTSQuerySessionInformationW = wtsapi32.NewProc("WTSQuerySessionInformationW")

	// WTSFreeMemory
	procWTSFreeMemory = wtsapi32.NewProc("WTSFreeMemory")

	// WTSEnumerateSessionsW
	procWTSEnumerateSessionsW = wtsapi32.NewProc("WTSEnumerateSessionsW")
)

// WTS_CONNECTSTATE_CLASS enum
const (
	WTSActive       = 0
	WTSConnected    = 1
	WTSConnectQuery = 2
	WTSShadow       = 3
	WTSDisconnected = 4
	WTSIdle         = 5
	WTSListen       = 6
	WTSReset        = 7
	WTSDown         = 8
	WTSInit         = 9
)

type WTS_SESSION_INFO struct {
	SessionId    uint32
	WinStationName *uint16
	State        uint32 // WTS_CONNECTSTATE_CLASS
}

func (a *agent) monitorRDP(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			active := isRDPActive()
			a.rdpActive.Store(active)
			if active {
				a.logger.Debug(ctx, "RDP session detected as active")
			}
		}
	}
}

func isRDPActive() bool {
	// Enumerate sessions to find active console or RDP sessions
	// We use WTS_CURRENT_SERVER_HANDLE (0)
	var sessionInfoPtr uintptr
	var count uint32

	// WTSEnumerateSessionsW(WTS_CURRENT_SERVER_HANDLE, 0, 1, &sessionInfoPtr, &count)
	ret, _, _ := procWTSEnumerateSessionsW.Call(
		0, // WTS_CURRENT_SERVER_HANDLE
		0, // Reserved
		1, // Version
		uintptr(unsafe.Pointer(&sessionInfoPtr)),
		uintptr(unsafe.Pointer(&count)),
	)

	if ret == 0 {
		return false
	}
	defer procWTSFreeMemory.Call(sessionInfoPtr)

	// Iterate over the sessions
	// struct size is 4 (id) + 8 (ptr) + 4 (state) = 16 bytes on 64-bit?
	// WTS_SESSION_INFO definition: DWORD SessionId; LPTSTR pWinStationName; WTS_CONNECTSTATE_CLASS State;
	// On 64-bit: 4 + 4(padding) + 8 + 4 + 4(padding) = 24 bytes?
	// Let's use unsafe.Sizeof logic if we could, but we can't easily.
	// Actually:
	// SessionId (4)
	// padding (4)
	// pWinStationName (8)
	// State (4)
	// padding (4)
	// Total 24 bytes.

	// Let's rely on constructing a slice from the pointer.
	// Note: this unsafe slice construction is standard for interacting with C arrays.

	// Determine struct size dynamically or just assume standard alignment.
	// For Go on Windows amd64, int is 64 bit? No, int is 64 bit, but WTS_SESSION_INFO uses DWORD (uint32).
	// pointer is 64 bit.
	// uint32 (4) -> align 8 -> 4 padding.
	// *uint16 (8)
	// uint32 (4) -> align 8 -> 4 padding.
	// Total 24 bytes.

    // on 32-bit it would be different.
	// We can use a helper struct to iterate.

	// Safer approach: iterate and decode.

	currentPtr := sessionInfoPtr
	// Size depends on arch.
	structSize := unsafe.Sizeof(WTS_SESSION_INFO{})

	for i := uint32(0); i < count; i++ {
		info := (*WTS_SESSION_INFO)(unsafe.Pointer(currentPtr))

		if info.State == WTSActive {
			// Check if it is RDP. 'Console' is local, 'RDP-Tcp#...' is RDP.
			// However, if we just want "someone is using the machine", Console is also valid activity!
			// But the goal is "RDP Keep Alive".
			// If I am locally logged in, workspace shouldn't shutdown either?
			// Usually yes.
			// The issue description specifically says "Windows RDP Keep Alive".
			// If I just check WTSActive, it covers both local console and RDP.
			// Let's filter by name if we want *only* RDP, or just accept any active session.
			// Standard "user activity" implies any active user session.

			// Let's assume WTSActive is enough to indicate "in use".

			// To be pedantic about RDP, we could check pWinStationName.
			name := windows.UTF16PtrToString(info.WinStationName)
			// RDP sessions usually start with "RDP-Tcp". Console is "Console".
			// If we want to support Console use too (e.g. VNC, physically attached), we should keep it.
			// But for the bounty context, let's include all WTSActive sessions.
			// Wait, the prompt says "Windows RDP Keep Alive".
			// If I am on Console, I am generating inputs that Windows *might* see, but maybe not if it's headless/cloud.
			// In a cloud VM (Coder), "Console" might be the web terminal or VNC?
			// Actually, Coder web terminal uses `agentexec`.
			// RDP creates a new session usually.
			// Let's return true if *any* session is WTSActive.

			// Actually, let's log the name for debug.
			// fmt.Printf("Found active session: %s\n", name)

			// Only consider it "activity" if it is Active (0).
			return true
		}

		currentPtr += structSize
	}

	return false
}
