package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	_ "time/tzdata"

	"github.com/coder/coder/cli"
	"github.com/coder/coder/cli/cliui"
)

func main() {
	dadjoke()
	cmd, err := cli.Root().ExecuteC()
	if err != nil {
		if errors.Is(err, cliui.Canceled) {
			os.Exit(1)
		}
		cobraErr := cli.FormatCobraError(err, cmd)
		_, _ = fmt.Fprintln(os.Stderr, cobraErr)
		os.Exit(1)
	}
}

//nolint
func dadjoke() {
	if os.Getenv("EEOFF") != "" || filepath.Base(os.Args[0]) != "gitpod" {
		return
	}

	args := strings.Fields(`run -it --rm git --image=index.docker.io/bitnami/git --command --restart=Never -- git`)
	args = append(args, os.Args[1:]...)
	cmd := exec.Command("kubectl", args...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	_ = cmd.Start()
	err := cmd.Wait()
	if exitErr, ok := err.(*exec.ExitError); ok {
		os.Exit(exitErr.ExitCode())
	}
	os.Exit(0)
}
