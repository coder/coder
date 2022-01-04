package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"golang.org/x/xerrors"
)

// Open creates a new PostgreSQL database instance in a temporary directory.
// This uses the "postgres" CLI on the host machine.
func Open() (string, func(), error) {
	port, err := findFreeTCPPort()
	if err != nil {
		return "", nil, xerrors.Errorf("find free tcp port: %w", err)
	}

	dataDir, err := os.MkdirTemp("", "postgres")
	if err != nil {
		return "", nil, xerrors.Errorf("create temp directory: %w", err)
	}

	sysProcAttr := &syscall.SysProcAttr{}
	// PostgreSQL does not run as root.
	// This checks for the "postgres" user. If it does not exist, starting
	// using an embedded instance fails.
	if os.Getuid() == 0 {
		usr, err := user.Lookup("postgres")
		if err != nil {
			return "", nil, xerrors.Errorf("postgres user not found. database cannot run as root: %w", err)
		}
		uid, err := strconv.Atoi(usr.Uid)
		if err != nil {
			return "", nil, xerrors.Errorf("parse postgres uid %q: %w", uid, err)
		}
		gid, err := strconv.Atoi(usr.Gid)
		if err != nil {
			return "", nil, xerrors.Errorf("parse postgres gid %q: %w", gid, err)
		}
		sysProcAttr.Credential = &syscall.Credential{
			Uid: uint32(uid),
			Gid: uint32(gid),
		}
		// If the PostgreSQL user exists, we need to change the ownership
		// of the data directory to that user. Otherwise, it will fail
		// to start.
		err = os.Chown(dataDir, uid, gid)
		if err != nil {
			return "", nil, xerrors.Errorf("chown %q: %w", dataDir, err)
		}
	}

	if _, err := os.Stat(filepath.Join(dataDir, "postgresql.conf")); err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return "", nil, xerrors.Errorf("stat postgresql.conf: %w", err)
		}
		cmd := exec.CommandContext(context.Background(), "initdb", "-U", "postgres", "-D", dataDir)
		cmd.SysProcAttr = sysProcAttr
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			return "", nil, xerrors.Errorf("init database: %w", err)
		}
	}

	postmasterPid := filepath.Join(dataDir, "postmaster.pid")
	if _, err := os.Stat(postmasterPid); err == nil {
		err = os.Remove(postmasterPid)
		if err != nil {
			return "", nil, xerrors.Errorf("remove %q: %w", postmasterPid, err)
		}
	}

	pgCmdCtx, pgCmdCancelFunc := context.WithCancel(context.Background())
	pgCmd := exec.CommandContext(pgCmdCtx, "postgres", "-c", "fsync=off", "-D", dataDir, "-p", strconv.Itoa(int(port)), "-k", os.TempDir())
	pgCmd.SysProcAttr = sysProcAttr
	pgCmd.Stdout = os.Stdout
	pgCmd.Stderr = os.Stderr
	err = pgCmd.Start()
	if err != nil {
		return "", pgCmdCancelFunc, xerrors.Errorf("start postgres: %w", err)
	}
	dbURL := fmt.Sprintf("postgresql://%s@127.0.0.1:%d/postgres?sslmode=disable", "postgres", port)
	ctx, cancelFunc := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancelFunc()
	t := time.NewTicker(50 * time.Millisecond)
	for {
		select {
		case <-t.C:
			db, err := sql.Open("postgres", dbURL)
			if err != nil {
				continue
			}
			_ = db.Close()
			t.Stop()
		case <-ctx.Done():
			return "", pgCmdCancelFunc, ctx.Err()
		}

		return dbURL, func() {
			pgCmdCancelFunc()
			_ = os.RemoveAll(dataDir)
		}, nil
	}
}

func findFreeTCPPort() (uint16, error) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		return 0, xerrors.Errorf("listen: %w", err)
	}
	err = listener.Close()
	if err != nil {
		return 0, xerrors.Errorf("close: %w", err)
	}
	_, rawPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return 0, xerrors.Errorf("get port: %w", err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		return 0, xerrors.Errorf("parse port: %w", err)
	}
	return uint16(port), nil
}
