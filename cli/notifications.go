package cli

import (
	"fmt"

	"github.com/coder/serpent"

	"github.com/coder/coder/v2/codersdk"
)

func (r *RootCmd) notifications() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "notifications",
		Short: "Manage Coder notifications",
		Long: "Administrators can use these commands to change notification settings.\n" + FormatExamples(
			Example{
				Description: "Pause Coder notifications",
				Command:     "coder notifications pause",
			},
			Example{
				Description: "Unpause Coder notifications",
				Command:     "coder notifications unpause",
			},
		),
		Aliases: []string{"notification"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.pauseNotifications(),
			r.unpauseNotifications(),
		},
	}
	return cmd
}

func (r *RootCmd) pauseNotifications() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "pause",
		Short: "Pause notifications",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			err := client.PutNotificationsSettings(inv.Context(), codersdk.NotificationsSettings{
				NotifierPaused: true,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Notifications are paused now.")
			return nil
		},
	}
	return cmd
}

func (r *RootCmd) unpauseNotifications() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "unpause",
		Short: "Unpause notifications",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			err := client.PutNotificationsSettings(inv.Context(), codersdk.NotificationsSettings{
				NotifierPaused: false,
			})
			if err != nil {
				return err
			}

			_, _ = fmt.Fprintln(inv.Stderr, "Notifications are unpaused now.")
			return nil
		},
	}
	return cmd
}
