package cli

import (
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/coder/coder/coderd"
	"github.com/coder/coder/codersdk"
	"github.com/fatih/color"
	"github.com/go-playground/validator/v10"
	"github.com/manifoldco/promptui"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"
)

func login() *cobra.Command {
	return &cobra.Command{
		Use:  "login <url>",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rawURL := args[0]
			if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
				scheme := "https"
				if strings.HasPrefix(rawURL, "localhost") {
					scheme = "http"
				}
				rawURL = fmt.Sprintf("%s://%s", scheme, rawURL)
			}
			serverURL, err := url.Parse(rawURL)
			if err != nil {
				return err
			}
			// Default to HTTPs. Enables simple URLs like: master.cdr.dev
			if serverURL.Scheme == "" {
				serverURL.Scheme = "https"
			}

			client := codersdk.New(serverURL)
			hasInitialUser, err := client.HasInitialUser(cmd.Context())
			if err != nil {
				return err
			}
			if !hasInitialUser {
				if !isTTY(cmd.InOrStdin()) {
					return xerrors.New("the initial user cannot be created in non-interactive mode. use the API")
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%s Your Coder deployment hasn't been setup!\n", color.HiBlackString(">"))

				_, err := runPrompt(cmd, &promptui.Prompt{
					Label:     "Would you like to create the first user?",
					IsConfirm: true,
					Default:   "y",
				})
				if err != nil {
					return err
				}

				username, err := runPrompt(cmd, &promptui.Prompt{
					Label:   "What username would you like?",
					Default: "kyle",
				})
				if err != nil {
					return err
				}

				organization, err := runPrompt(cmd, &promptui.Prompt{
					Label:   "What is the name of your organization?",
					Default: "acme-corp",
				})
				if err != nil {
					return err
				}

				email, err := runPrompt(cmd, &promptui.Prompt{
					Label: "What's your email?",
					Validate: func(s string) error {
						err := validator.New().Var(s, "email")
						if err != nil {
							return errors.New("That's not a valid email address!")
						}
						return err
					},
				})
				if err != nil {
					return err
				}

				password, err := runPrompt(cmd, &promptui.Prompt{
					Label: "Enter a password:",
					Mask:  '*',
				})
				if err != nil {
					return err
				}

				_, err = client.CreateInitialUser(cmd.Context(), coderd.CreateInitialUserRequest{
					Email:        email,
					Username:     username,
					Password:     password,
					Organization: organization,
				})
				if err != nil {
					return xerrors.Errorf("create initial user: %w", err)
				}
				resp, err := client.LoginWithPassword(cmd.Context(), coderd.LoginWithPasswordRequest{
					Email:    email,
					Password: password,
				})
				if err != nil {
					return xerrors.Errorf("login with password: %w", err)
				}
				config := createConfig(cmd)
				err = config.Session().Write(resp.SessionToken)
				if err != nil {
					return xerrors.Errorf("write session token: %w", err)
				}
				err = config.URL().Write(serverURL.String())
				if err != nil {
					return xerrors.Errorf("write server url: %w", err)
				}

				fmt.Fprintf(cmd.OutOrStdout(), "%s Welcome to Coder, %s! You're logged in.\n", color.HiBlackString(">"), color.HiCyanString(username))
				return nil
			}

			return nil
		},
	}
}
