package cli
import (
	"errors"
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
				Description: "Pause Coder notifications. Administrators can temporarily stop notifiers from dispatching messages in case of the target outage (for example: unavailable SMTP server or Webhook not responding).",
				Command:     "coder notifications pause",
			},
			Example{
				Description: "Resume Coder notifications",
				Command:     "coder notifications resume",
			},
			Example{
				Description: "Send a test notification. Administrators can use this to verify the notification target settings.",
				Command:     "coder notifications test",
			},
		),
		Aliases: []string{"notification"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.pauseNotifications(),
			r.resumeNotifications(),
			r.testNotifications(),
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
				return fmt.Errorf("unable to pause notifications: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stderr, "Notifications are now paused.")
			return nil
		},
	}
	return cmd
}
func (r *RootCmd) resumeNotifications() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "resume",
		Short: "Resume notifications",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			err := client.PutNotificationsSettings(inv.Context(), codersdk.NotificationsSettings{
				NotifierPaused: false,
			})
			if err != nil {
				return fmt.Errorf("unable to resume notifications: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stderr, "Notifications are now resumed.")
			return nil
		},
	}
	return cmd
}
func (r *RootCmd) testNotifications() *serpent.Command {
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use:   "test",
		Short: "Send a test notification",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			if err := client.PostTestNotification(inv.Context()); err != nil {
				return fmt.Errorf("unable to post test notification: %w", err)
			}
			_, _ = fmt.Fprintln(inv.Stderr, "A test notification has been sent. If you don't receive the notification, check Coder's logs for any errors.")
			return nil
		},
	}
	return cmd
}
