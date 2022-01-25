package cmd

import (
	"errors"

	"github.com/spf13/cobra"
)

func Root() *cobra.Command {
	return &cobra.Command{
		Use: "provisionerd",
		RunE: func(cmd *cobra.Command, args []string) error {
			return errors.New("not implemented")
		},
	}
}
