package cli

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"os/user"
	"path"
	"runtime"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/pkg/browser"
	"golang.org/x/xerrors"

	"github.com/coder/pretty"

	"github.com/coder/coder/v2/cli/clibase"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/codersdk"
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
	browser.Stderr = io.Discard
	browser.Stdout = io.Discard
}

func promptFirstUsername(inv *clibase.Invocation) (string, error) {
	currentUser, err := user.Current()
	if err != nil {
		return "", xerrors.Errorf("get current user: %w", err)
	}
	username, err := cliui.Prompt(inv, cliui.PromptOptions{
		Text:    "What " + pretty.Sprint(cliui.DefaultStyles.Field, "username") + " would you like?",
		Default: currentUser.Username,
	})
	if errors.Is(err, cliui.Canceled) {
		return "", nil
	}
	if err != nil {
		return "", err
	}

	return username, nil
}

func promptFirstPassword(inv *clibase.Invocation) (string, error) {
retry:
	password, err := cliui.Prompt(inv, cliui.PromptOptions{
		Text:   "Enter a " + pretty.Sprint(cliui.DefaultStyles.Field, "password") + ":",
		Secret: true,
		Validate: func(s string) error {
			return userpassword.Validate(s)
		},
	})
	if err != nil {
		return "", xerrors.Errorf("specify password prompt: %w", err)
	}
	confirm, err := cliui.Prompt(inv, cliui.PromptOptions{
		Text:     "Confirm " + pretty.Sprint(cliui.DefaultStyles.Field, "password") + ":",
		Secret:   true,
		Validate: cliui.ValidateNotEmpty,
	})
	if err != nil {
		return "", xerrors.Errorf("confirm password prompt: %w", err)
	}

	if confirm != password {
		_, _ = fmt.Fprintln(inv.Stdout, pretty.Sprint(cliui.DefaultStyles.Error, "Passwords do not match"))
		goto retry
	}

	return password, nil
}

func (r *RootCmd) loginWithPassword(
	inv *clibase.Invocation,
	client *codersdk.Client,
	email, password string,
) error {
	resp, err := client.LoginWithPassword(inv.Context(), codersdk.LoginWithPasswordRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		return xerrors.Errorf("login with password: %w", err)
	}

	sessionToken := resp.SessionToken
	config := r.createConfig()
	err = config.Session().Write(sessionToken)
	if err != nil {
		return xerrors.Errorf("write session token: %w", err)
	}

	client.SetSessionToken(sessionToken)

	// Nice side-effect: validates the token.
	u, err := client.User(inv.Context(), "me")
	if err != nil {
		return xerrors.Errorf("get user: %w", err)
	}

	_, _ = fmt.Fprintf(
		inv.Stdout,
		"Welcome to Coder, %s! You're authenticated.",
		pretty.Sprint(cliui.DefaultStyles.Keyword, u.Username),
	)

	return nil
}

func (r *RootCmd) login() *clibase.Cmd {
	const firstUserTrialEnv = "CODER_FIRST_USER_TRIAL"

	var (
		email              string
		username           string
		password           string
		trial              bool
		useTokenForSession bool
	)
	cmd := &clibase.Cmd{
		Use:        "login <url>",
		Short:      "Authenticate with Coder deployment",
		Middleware: clibase.RequireRangeArgs(0, 1),
		Handler: func(inv *clibase.Invocation) error {
			ctx := inv.Context()
			rawURL := ""
			if len(inv.Args) == 0 {
				rawURL = r.clientURL.String()
			} else {
				rawURL = inv.Args[0]
			}

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

			client, err := r.createUnauthenticatedClient(ctx, serverURL)
			if err != nil {
				return err
			}

			// Try to check the version of the server prior to logging in.
			// It may be useful to warn the user if they are trying to login
			// on a very old client.
			err = r.checkVersions(inv, client)
			if err != nil {
				// Checking versions isn't a fatal error so we print a warning
				// and proceed.
				_, _ = fmt.Fprintln(inv.Stderr, pretty.Sprint(cliui.DefaultStyles.Warn, err.Error()))
			}

			hasFirstUser, err := client.HasFirstUser(ctx)
			if err != nil {
				return xerrors.Errorf("Failed to check server %q for first user, is the URL correct and is coder accessible from your browser? Error - has initial user: %w", serverURL.String(), err)
			}
			if !hasFirstUser {
				_, _ = fmt.Fprintf(inv.Stdout, Caret+"Your Coder deployment hasn't been set up!\n")

				if username == "" {
					if !isTTY(inv) {
						return xerrors.New("the initial user cannot be created in non-interactive mode. use the API")
					}

					_, err := cliui.Prompt(inv, cliui.PromptOptions{
						Text:      "Would you like to create the first user?",
						Default:   cliui.ConfirmYes,
						IsConfirm: true,
					})
					if err != nil {
						return err
					}

					username, err = promptFirstUsername(inv)
					if err != nil {
						return err
					}
				}

				if email == "" {
					email, err = cliui.Prompt(inv, cliui.PromptOptions{
						Text: "What's your " + pretty.Sprint(cliui.DefaultStyles.Field, "email") + "?",
						Validate: func(s string) error {
							err := validator.New().Var(s, "email")
							if err != nil {
								return xerrors.New("That's not a valid email address!")
							}
							return err
						},
					})
					if err != nil {
						return err
					}
				}

				if password == "" {
					password, err = promptFirstPassword(inv)
					if err != nil {
						return err
					}
				}

				if !inv.ParsedFlags().Changed("first-user-trial") && os.Getenv(firstUserTrialEnv) == "" {
					v, _ := cliui.Prompt(inv, cliui.PromptOptions{
						Text:      "Start a 30-day trial of Enterprise?",
						IsConfirm: true,
						Default:   "yes",
					})
					trial = v == "yes" || v == "y"
				}

				_, err = client.CreateFirstUser(ctx, codersdk.CreateFirstUserRequest{
					Email:    email,
					Username: username,
					Password: password,
					Trial:    trial,
				})
				if err != nil {
					return xerrors.Errorf("create initial user: %w", err)
				}

				err := r.loginWithPassword(inv, client, email, password)
				if err != nil {
					return err
				}

				err = r.createConfig().URL().Write(serverURL.String())
				if err != nil {
					return xerrors.Errorf("write server url: %w", err)
				}

				_, _ = fmt.Fprintf(
					inv.Stdout,
					"Get started by creating a template: %s\n",
					pretty.Sprint(cliui.DefaultStyles.Code, "coder templates init"),
				)
				return nil
			}

			sessionToken, _ := inv.ParsedFlags().GetString(varToken)
			if sessionToken == "" {
				authURL := *serverURL
				// Don't use filepath.Join, we don't want to use the os separator
				// for a url.
				authURL.Path = path.Join(serverURL.Path, "/cli-auth")
				if err := openURL(inv, authURL.String()); err != nil {
					_, _ = fmt.Fprintf(inv.Stdout, "Open the following in your browser:\n\n\t%s\n\n", authURL.String())
				} else {
					_, _ = fmt.Fprintf(inv.Stdout, "Your browser has been opened to visit:\n\n\t%s\n\n", authURL.String())
				}

				sessionToken, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Paste your token here:",
					Validate: func(token string) error {
						client.SetSessionToken(token)
						_, err := client.User(ctx, codersdk.Me)
						if err != nil {
							return xerrors.New("That's not a valid token!")
						}
						return err
					},
				})
				if err != nil {
					return xerrors.Errorf("paste token prompt: %w", err)
				}
			} else if !useTokenForSession {
				// If a session token is provided on the cli, use it to generate
				// a new one. This is because the cli `--token` flag provides
				// a token for the command being invoked. We should not store
				// this token, and `/logout` should not delete it.
				// /login should generate a new token and store that.
				client.SetSessionToken(sessionToken)
				// Use CreateAPIKey over CreateToken because this is a session
				// key that should not show on the `tokens` page. This should
				// match the same behavior of the `/cli-auth` page for generating
				// a session token.
				key, err := client.CreateAPIKey(ctx, "me")
				if err != nil {
					return xerrors.Errorf("create api key: %w", err)
				}
				sessionToken = key.Key
			}

			// Login to get user data - verify it is OK before persisting
			client.SetSessionToken(sessionToken)
			resp, err := client.User(ctx, codersdk.Me)
			if err != nil {
				return xerrors.Errorf("get user: %w", err)
			}

			config := r.createConfig()
			err = config.Session().Write(sessionToken)
			if err != nil {
				return xerrors.Errorf("write session token: %w", err)
			}
			err = config.URL().Write(serverURL.String())
			if err != nil {
				return xerrors.Errorf("write server url: %w", err)
			}

			_, _ = fmt.Fprintf(inv.Stdout, Caret+"Welcome to Coder, %s! You're authenticated.\n", pretty.Sprint(cliui.DefaultStyles.Keyword, resp.Username))
			return nil
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:        "first-user-email",
			Env:         "CODER_FIRST_USER_EMAIL",
			Description: "Specifies an email address to use if creating the first user for the deployment.",
			Value:       clibase.StringOf(&email),
		},
		{
			Flag:        "first-user-username",
			Env:         "CODER_FIRST_USER_USERNAME",
			Description: "Specifies a username to use if creating the first user for the deployment.",
			Value:       clibase.StringOf(&username),
		},
		{
			Flag:        "first-user-password",
			Env:         "CODER_FIRST_USER_PASSWORD",
			Description: "Specifies a password to use if creating the first user for the deployment.",
			Value:       clibase.StringOf(&password),
		},
		{
			Flag:        "first-user-trial",
			Env:         firstUserTrialEnv,
			Description: "Specifies whether a trial license should be provisioned for the Coder deployment or not.",
			Value:       clibase.BoolOf(&trial),
		},
		{
			Flag:        "use-token-as-session",
			Description: "By default, the CLI will generate a new session token when logging in. This flag will instead use the provided token as the session token.",
			Value:       clibase.BoolOf(&useTokenForSession),
		},
	}
	return cmd
}

// isWSL determines if coder-cli is running within Windows Subsystem for Linux
func isWSL() (bool, error) {
	if runtime.GOOS == goosDarwin || runtime.GOOS == goosWindows {
		return false, nil
	}
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return false, xerrors.Errorf("read /proc/version: %w", err)
	}
	return strings.Contains(strings.ToLower(string(data)), "microsoft"), nil
}

// openURL opens the provided URL via user's default browser
func openURL(inv *clibase.Invocation, urlToOpen string) error {
	noOpen, err := inv.ParsedFlags().GetBool(varNoOpen)
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

	browserEnv := os.Getenv("BROWSER")
	if browserEnv != "" {
		browserSh := fmt.Sprintf("%s '%s'", browserEnv, urlToOpen)
		cmd := exec.CommandContext(inv.Context(), "sh", "-c", browserSh)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return xerrors.Errorf("failed to run %v (out: %q): %w", cmd.Args, out, err)
		}
		return nil
	}

	return browser.OpenURL(urlToOpen)
}
