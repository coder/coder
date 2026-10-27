package filefinder

import "os/exec"

// killProcessGroupOnCancel is a no-op on Windows; cmd.WaitDelay still
// bounds waiting for output pipes held by descendants.
func killProcessGroupOnCancel(*exec.Cmd) {}
