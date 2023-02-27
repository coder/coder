//go:build slim

package cli

import (
	"context"
	"io"

	"golang.org/x/xerrors"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
	"github.com/spf13/cobra"
)

func server() *cobra.Command {
	cmd := agpl.Server(func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, io.Closer, error) {
		return nil, nil, xerrors.Errorf("slim build does not support `coder server`")
	})
	return cmd
}
