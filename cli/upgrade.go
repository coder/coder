package cli

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"

	"github.com/coder/coder/cli/clibase"
)

func (r *RootCmd) upgrade() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:   "upgrade",
		Short: "Upgrade the Coder CLI to match the version of a deployment.",
		Handler: func(inv *clibase.Invocation) error {

			resp, err := http.Get("https://coder.com/install.sh")
			if err != nil {
				panic(err)
			}
			if resp.StatusCode != http.StatusOK {
				panic(fmt.Sprintf("code %d", resp.StatusCode))
			}
			defer resp.Body.Close()

			script, err := io.ReadAll(resp.Body)
			if err != nil {
				panic(err)
			}
			fmt.Println("script: ", string(script))

			cmd := exec.Command("sh", "-s", "--", "--version", "0.22.2")
			cmd.Stdin = bytes.NewReader(script)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			err = cmd.Run()
			if err != nil {
				panic(err)
			}

			return nil
		},
	}

	return cmd
}
