package cli

import (
	"errors"
	"fmt"
	"io/ioutil"
	"net/url"
	"os/exec"
	"os/user"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/fatih/color"
	"github.com/go-playground/validator/v10"
	"github.com/manifoldco/promptui"
	"github.com/pkg/browser"
	"github.com/spf13/cobra"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

const (
	goosWindows = "windows"
	goosDarwin  = "darwin"
)

func init() {
	// Hide output from the browser library,
	// otherwise we can get really verbose and non-actionable messages
	// when in SSH or another type of headless session
	// NOTE: This needs to be in `init` to prevent data races
	// (multiple threads trying to set the global browser.Std* variables)
	browser.Stderr = ioutil.Discard
	browser.Stdout = ioutil.Discard
}

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
				return xerrors.Errorf("parse raw url %q: %w", rawURL, err)
			}
			// Default to HTTPs. Enables simple URLs like: master.cdr.dev
			if serverURL.Scheme == "" {
				serverURL.Scheme = "https"
			}

			client := codersdk.New(serverURL)
			hasInitialUser, err := client.HasFirstUser(cmd.Context())
			if err != nil {
				return xerrors.Errorf("has initial user: %w", err)
			}
			if !hasInitialUser {
				if !isTTY(cmd) {
					return xerrors.New("the initial user cannot be created in non-interactive mode. use the API")
				}
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Your Coder deployment hasn't been set up!\n", caret)

				_, err := cliui.Prompt(cmd, cliui.PromptOptions{
					Prompt:    "Would you like to create the first user?",
					Default:   "yes",
					IsConfirm: true,
				})
				if errors.Is(err, cliui.Canceled) {
					return nil
				}
				if err != nil {
					return err
				}
				currentUser, err := user.Current()
				if err != nil {
					return xerrors.Errorf("get current user: %w", err)
				}
				username, err := cliui.Prompt(cmd, cliui.PromptOptions{
					Prompt:  "What " + cliui.Styles.Field.Render("username") + " would you like?",
					Default: currentUser.Username,
				})
				if errors.Is(err, cliui.Canceled) {
					return nil
				}
				if err != nil {
					return xerrors.Errorf("pick username prompt: %w", err)
				}

				email, err := cliui.Prompt(cmd, cliui.PromptOptions{
					Prompt: "What's your " + cliui.Styles.Field.Render("email") + "?",
					Validate: func(s string) error {
						err := validator.New().Var(s, "email")
						if err != nil {
							return xerrors.New("That's not a valid email address!")
						}
						return err
					},
				})
				if err != nil {
					return xerrors.Errorf("specify email prompt: %w", err)
				}

				password, err := cliui.Prompt(cmd, cliui.PromptOptions{
					Prompt:        "Enter a " + cliui.Styles.Field.Render("password") + ":",
					EchoMode:      textinput.EchoPassword,
					EchoCharacter: '*',
					Validate:      cliui.ValidateNotEmpty,
				})
				if err != nil {
					return xerrors.Errorf("specify password prompt: %w", err)
				}

				_, err = client.CreateFirstUser(cmd.Context(), codersdk.CreateFirstUserRequest{
					Email:        email,
					Username:     username,
					Organization: username,
					Password:     password,
				})
				if err != nil {
					return xerrors.Errorf("create initial user: %w", err)
				}
				resp, err := client.LoginWithPassword(cmd.Context(), codersdk.LoginWithPasswordRequest{
					Email:    email,
					Password: password,
				})
				if err != nil {
					return xerrors.Errorf("login with password: %w", err)
				}

				sessionToken := resp.SessionToken
				config := createConfig(cmd)
				err = config.Session().Write(sessionToken)
				if err != nil {
					return xerrors.Errorf("write session token: %w", err)
				}
				err = config.URL().Write(serverURL.String())
				if err != nil {
					return xerrors.Errorf("write server url: %w", err)
				}

				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Welcome to Coder, %s! You're authenticated.\n", caret, color.HiCyanString(username))
				return nil
			}

			authURL := *serverURL
			authURL.Path = serverURL.Path + "/cli-auth"
			if err := openURL(cmd, authURL.String()); err != nil {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Open the following in your browser:\n\n\t%s\n\n", authURL.String())
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Your browser has been opened to visit:\n\n\t%s\n\n", authURL.String())
			}

			sessionToken, err := prompt(cmd, &promptui.Prompt{
				Label: "Paste your token here:",
				Mask:  '*',
				Validate: func(token string) error {
					client.SessionToken = token
					_, err := client.User(cmd.Context(), "me")
					if err != nil {
						return xerrors.New("That's not a valid token!")
					}
					return err
				},
			})
			if err != nil {
				return xerrors.Errorf("paste token prompt: %w", err)
			}

			// Login to get user data - verify it is OK before persisting
			client.SessionToken = sessionToken
			resp, err := client.User(cmd.Context(), "me")
			if err != nil {
				return xerrors.Errorf("get user: %w", err)
			}

			config := createConfig(cmd)
			err = config.Session().Write(sessionToken)
			if err != nil {
				return xerrors.Errorf("write session token: %w", err)
			}
			err = config.URL().Write(serverURL.String())
			if err != nil {
				return xerrors.Errorf("write server url: %w", err)
			}

			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s Welcome to Coder, %s! You're authenticated.\n", caret, color.HiCyanString(resp.Username))
			return nil
		},
	}
}

// isWSL determines if coder-cli is running within Windows Subsystem for Linux
func isWSL() (bool, error) {
	if runtime.GOOS == goosDarwin || runtime.GOOS == goosWindows {
		return false, nil
	}
	data, err := ioutil.ReadFile("/proc/version")
	if err != nil {
		return false, xerrors.Errorf("read /proc/version: %w", err)
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft"), nil
}

// openURL opens the provided URL via user's default browser
func openURL(cmd *cobra.Command, urlToOpen string) error {
	noOpen, err := cmd.Flags().GetBool(varNoOpen)
	if err != nil {
		panic(err)
	}
	if noOpen {
		return xerrors.New("opening is blocked")
	}
	wsl, err := isWSL()
	if err != nil {
		return xerrors.Errorf("test running Windows Subsystem for Linux: %w", err)
	}

	if wsl {
		// #nosec
		return exec.Command("cmd.exe", "/c", "start", strings.ReplaceAll(urlToOpen, "&", "^&")).Start()
	}

	return browser.OpenURL(urlToOpen)
}
