//go:build slim

package cli

import (
	"context"
	"io"

	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	agplcoderd "github.com/coder/coder/coderd"
)

func (r *RootCmd) server() *clibase.Cmd {
	cmd := r.Server(func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, io.Closer, error) {
		return nil, nil, xerrors.Errorf("slim build does not support `coder server`")
	})
	return cmd
}
