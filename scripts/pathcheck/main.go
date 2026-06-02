package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func main() {
	for _, e := range os.Environ() {
		k, v, _ := strings.Cut(e, "=")
		if strings.EqualFold(k, "PATH") {
			entries := strings.Split(v, ";")
			if len(entries) <= 1 {
				entries = strings.Split(v, ":")
			}
			_, _ = fmt.Printf("Go %s (%d entries, first 5):\n", k, len(entries))
			for i, p := range entries {
				if i >= 5 {
					break
				}
				_, _ = fmt.Printf("  [%d] %s\n", i, p)
			}
		}
	}

	// Reproduce exactly what headerTransport does.
	cmd := exec.Command("cmd.exe", "/c", "printf test-from-go")
	cmd.Env = append(os.Environ(), "CODER_URL=http://test")
	// headerTransport sets: cmd.Stdout = &outBuf, cmd.Stderr = io.Discard
	out, err := cmd.Output()
	_, _ = fmt.Printf("cmd.exe /c printf (with Env): out=%q err=%v\n", string(out), err)

	// Also try without setting Env (inherit parent).
	cmd2 := exec.Command("cmd.exe", "/c", "printf test-inherit")
	out2, err2 := cmd2.Output()
	_, _ = fmt.Printf("cmd.exe /c printf (inherit):  out=%q err=%v\n", string(out2), err2)
}
