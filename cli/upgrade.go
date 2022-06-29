package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

func upgrade() *cobra.Command {

	cmd := &cobra.Command{
		Use:   "upgrade",
		Short: "Upgrade coder CLI to server version.",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			client, err := createClient(cmd)
			if err != nil {
				return xerrors.Errorf("create client: %w", err)
			}

			bin, err := client.Upgrade(ctx)
			if err != nil {
				return xerrors.Errorf("download binary: %w", err)
			}
			defer bin.Close()

			exe, err := os.Executable()
			if err != nil {
				return xerrors.Errorf("get local executable: %w", err)
			}

			stat, err := os.Stat(exe)
			if err != nil {
				return xerrors.Errorf("stat %q: %w", exe, err)
			}

			// We intentionally do not use os.TmpDir here since it may
			// give us an 'invalid cross-device link' error.
			dir := filepath.Dir(exe)
			tmpPath := filepath.Join(dir, fmt.Sprintf(".coder-%v", time.Now().Unix()))
			tmpFi, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, stat.Mode().Perm())
			if err != nil {
				return xerrors.Errorf("create temp file %q: %w", tmpPath, err)
			}
			defer tmpFi.Close()

			_, err = io.Copy(tmpFi, bin)
			if err != nil {
				return xerrors.Errorf("copy binary: %w", err)
			}

			err = tmpFi.Close()
			if err != nil {
				return xerrors.Errorf("close temp file %q: %w", tmpPath, err)
			}

			err = os.Rename(tmpPath, exe)
			if err != nil {
				return xerrors.Errorf("rename %q to %q: %w", tmpPath, exe, err)
			}

			return nil
		},
	}

	return cmd
}
