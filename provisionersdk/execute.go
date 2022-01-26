package provisionersdk

import (
	"context"
	"os"
	"os/exec"

	"golang.org/x/xerrors"
	"storj.io/drpc/drpcconn"

	"github.com/coder/coder/provisionersdk/proto"
)

func Execute(ctx context.Context, binaryPath string) (proto.DRPCProvisionerClient, error) {
	file, err := os.Stat(binaryPath)
	if err != nil {
		return nil, xerrors.Errorf("stat %q: %w", binaryPath, err)
	}
	if file.Mode()&0111 == 0 {
		return nil, xerrors.Errorf("%q is not executable", binaryPath)
	}
	ctx, cancelFunc := context.WithCancel(ctx)
	cmd := exec.CommandContext(ctx, binaryPath)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancelFunc()
		return nil, xerrors.Errorf("stdin: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancelFunc()
		return nil, xerrors.Errorf("stdout: %w", err)
	}
	err = cmd.Start()
	if err != nil {
		cancelFunc()
		return nil, xerrors.Errorf("start %q: %w", binaryPath, err)
	}
	transport := &transport{
		in:  stdout,
		out: stdin,
		close: func() {
			_ = cmd.Process.Kill()
			cancelFunc()
		},
	}

	return proto.NewDRPCProvisionerClient(drpcconn.New(transport)), nil
}
