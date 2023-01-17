//go:build slim

package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/cli/deployment"
	"github.com/coder/coder/coderd"
)

func Server(vip *viper.Viper, _ func(context.Context, *coderd.Options) (*coderd.API, io.Closer, error)) *cobra.Command {
	root := &cobra.Command{
		Use:    "server",
		Short:  "Start a Coder server",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			serverUnsupported(cmd.ErrOrStderr())
			return nil
		},
	}

	var pgRawURL bool
	postgresBuiltinURLCmd := &cobra.Command{
		Use:    "postgres-builtin-url",
		Short:  "Output the connection URL for the built-in PostgreSQL deployment.",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			serverUnsupported(cmd.ErrOrStderr())
			return nil
		},
	}
	postgresBuiltinServeCmd := &cobra.Command{
		Use:    "postgres-builtin-serve",
		Short:  "Run the built-in PostgreSQL deployment.",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			serverUnsupported(cmd.ErrOrStderr())
			return nil
		},
	}

	// We still have to attach the flags to the commands so users don't get
	// an error when they try to use them.
	postgresBuiltinURLCmd.Flags().BoolVar(&pgRawURL, "raw-url", false, "Output the raw connection URL instead of a psql command.")
	postgresBuiltinServeCmd.Flags().BoolVar(&pgRawURL, "raw-url", false, "Output the raw connection URL instead of a psql command.")

	root.AddCommand(postgresBuiltinURLCmd, postgresBuiltinServeCmd)

	deployment.AttachFlags(root.Flags(), vip, false)

	return root
}

func serverUnsupported(w io.Writer) {
	_, _ = fmt.Fprintf(w, "You are using a 'slim' build of Coder, which does not support the %s subcommand.\n", cliui.Styles.Code.Render("server"))
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Please use a build of Coder from GitHub releases:")
	_, _ = fmt.Fprintln(w, "  https://github.com/coder/coder/releases")
	os.Exit(1)
}
