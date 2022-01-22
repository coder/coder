package cmd

import (
	"net"
	"net/http"
	"os"

	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/coderd"
	"github.com/coder/coder/database/databasefake"
)

func Root() *cobra.Command {
	var (
		address string
	)
	root := &cobra.Command{
		Use: "coderd",
		RunE: func(cmd *cobra.Command, args []string) error {
			handler := coderd.New(&coderd.Options{
				Logger:   slog.Make(sloghuman.Sink(os.Stderr)),
				Database: databasefake.New(),
			})

			listener, err := net.Listen("tcp", address)
			if err != nil {
				return xerrors.Errorf("listen %q: %w", address, err)
			}
			defer listener.Close()

			errCh := make(chan error)
			go func() {
				defer close(errCh)
				errCh <- http.Serve(listener, handler)
			}()

			select {
			case <-cmd.Context().Done():
				return cmd.Context().Err()
			case err := <-errCh:
				return err
			}
		},
	}
	defaultAddress, ok := os.LookupEnv("ADDRESS")
	if !ok {
		defaultAddress = "127.0.0.1:3000"
	}
	root.Flags().StringVarP(&address, "address", "a", defaultAddress, "The address to serve the API and dashboard.")

	return root
}
