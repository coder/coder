package cli

import (
	"context"

	"github.com/spf13/cobra"

	agpl "github.com/coder/coder/cli"
	agplcoderd "github.com/coder/coder/coderd"
	"github.com/coder/coder/enterprise/coderd"
)

func enterpriseOnly() []*cobra.Command {
	return []*cobra.Command{
		agpl.Server(func(ctx context.Context, options *agplcoderd.Options) (*agplcoderd.API, error) {
			api, err := coderd.New(ctx, &coderd.Options{
				Options: options,
			})
			if err != nil {
				return nil, err
			}
			return api.AGPL, nil
		}),
		features(),
		licenses(),
	}
}

func EnterpriseSubcommands() []*cobra.Command {
	all := append(agpl.Core(), enterpriseOnly()...)
	return all
}
