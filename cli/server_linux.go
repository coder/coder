//go:build linux

package cli

import (
	"context"
	"database/sql"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/config"
)

func startBuiltinPostgresAs(ctx context.Context, uid, gid uint32, stdout, stderr io.Writer, cfg config.Root, connectionURL string) (closer func() error, err error) {
	cmd := exec.Command(os.Args[0], "server", "postgres-builtin-serve") //nolint:gosec
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	// The postgres-builtin-serve command only uses the global config dir.
	cmd.Env = append(cmd.Env, "CODER_CONFIG_DIR="+string(cfg))

	cmd.SysProcAttr = &syscall.SysProcAttr{
		// Drop privileges.
		Credential: &syscall.Credential{
			Uid: uid,
			Gid: gid,
		},
		// Prevent signal propagation via
		// process group (handle manually).
		Setpgid: true,
		Pgid:    0,
	}

	// Fix privileges for the postgres directory, note that this will
	// won't help if the coder config is located inside e.g. /root
	// because the unprivileged user won't be able to access anything
	// inside.
	err = filepath.WalkDir(cfg.PostgresPath(), func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		err = os.Chown(path, int(uid), int(gid))
		if err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, xerrors.Errorf("start built-in PostgreSQL: %w", err)
	}
	errC := make(chan error, 1)
	go func() {
		errC <- cmd.Wait()
		close(errC)
	}()

	closePg := func() error {
		err := cmd.Process.Signal(os.Interrupt)
		if err != nil {
			return err
		}
		err = <-errC
		if err != nil {
			return err
		}
		return nil
	}
	defer func() {
		if err != nil {
			_ = closePg()
		}
	}()

	// Wait for PostgreSQL to be ready.
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case err = <-errC:
			return nil, xerrors.Errorf("built-in PostgreSQL exited: %w", err)
		default:
		}

		ok, err := func() (bool, error) {
			sqlDB, err := sql.Open("postgres", connectionURL)
			if err != nil {
				return false, err
			}
			defer sqlDB.Close()
			err = sqlDB.PingContext(ctx)
			if err != nil {
				return false, err
			}
			return true, nil
		}()
		if xerrors.Is(err, context.Canceled) {
			return nil, err
		}
		if ok {
			break
		}
		if i >= 100 { // 50 * 100ms = 5s.
			return nil, xerrors.Errorf("wait for built-in PostgreSQL timeout: %w", err)
		}
		time.Sleep(50 * time.Millisecond)
	}

	return closePg, nil
}
