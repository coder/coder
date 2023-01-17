//go:build slim

package cli

import (
	"context"
	"io"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/deployment"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
)

func server() *cobra.Command {
	vip := deployment.NewViper()
	cmd := agpl.Server(vip, func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, io.Closer, error) {
		return nil, nil, xerrors.Errorf("slim build does not support `coder server`")
	})

	deployment.AttachFlags(cmd.Flags(), vip, true)

	return cmd
}
