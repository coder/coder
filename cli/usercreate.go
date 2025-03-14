package cli
import (
	"errors"
	"fmt"
	"strings"
	"github.com/go-playground/validator/v10"
	"github.com/google/uuid"
	"github.com/coder/pretty"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/serpent"
)
func (r *RootCmd) userCreate() *serpent.Command {
	var (
		email        string
		username     string
		name         string
		password     string
		disableLogin bool
		loginType    string
		orgContext   = NewOrganizationContext()
	)
	client := new(codersdk.Client)
	cmd := &serpent.Command{
		Use: "create",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *serpent.Invocation) error {
			organization, err := orgContext.Selected(inv, client)
			if err != nil {
				return err
			}
			// We only prompt for the full name if both username and email have not
			// been set. This is to avoid breaking existing non-interactive usage.
			shouldPromptName := username == "" && email == ""
			if username == "" {
				username, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Username:",
					Validate: func(username string) error {
						err = codersdk.NameValid(username)
						if err != nil {
							return fmt.Errorf("username %q is invalid: %w", username, err)
						}
						return nil
					},
				})
				if err != nil {
					return err
				}
			}
			if email == "" {
				email, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Email:",
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
			}
			if name == "" && shouldPromptName {
				rawName, err := cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Full name (optional):",
				})
				if err != nil {
					return err
				}
				name = codersdk.NormalizeRealUsername(rawName)
				if !strings.EqualFold(rawName, name) {
					cliui.Warnf(inv.Stderr, "Normalized name to %q", name)
				}
			}
			userLoginType := codersdk.LoginTypePassword
			if disableLogin && loginType != "" {
				return errors.New("You cannot specify both --disable-login and --login-type")
			}
			if disableLogin {
				userLoginType = codersdk.LoginTypeNone
			} else if loginType != "" {
				userLoginType = codersdk.LoginType(loginType)
			}
			if password == "" && userLoginType == codersdk.LoginTypePassword {
				// Generate a random password
				password, err = cryptorand.StringCharset(cryptorand.Human, 20)
				if err != nil {
					return err
				}
			}
			_, err = client.CreateUserWithOrgs(inv.Context(), codersdk.CreateUserRequestWithOrgs{
				Email:           email,
				Username:        username,
				Name:            name,
				Password:        password,
				OrganizationIDs: []uuid.UUID{organization.ID},
				UserLoginType:   userLoginType,
			})
			if err != nil {
				return err
			}
			authenticationMethod := ""
			switch codersdk.LoginType(strings.ToLower(string(userLoginType))) {
			case codersdk.LoginTypePassword:
				authenticationMethod = `Your password is: ` + pretty.Sprint(cliui.DefaultStyles.Field, password)
			case codersdk.LoginTypeNone:
				authenticationMethod = "Login has been disabled for this user. Contact your administrator to authenticate."
			case codersdk.LoginTypeGithub:
				authenticationMethod = `Login is authenticated through GitHub.`
			case codersdk.LoginTypeOIDC:
				authenticationMethod = `Login is authenticated through the configured OIDC provider.`
			}
			_, _ = fmt.Fprintln(inv.Stderr, `A new user has been created!
Share the instructions below to get them started.
`+pretty.Sprint(cliui.DefaultStyles.Placeholder, "—————————————————————————————————————————————————")+`
Download the Coder command line for your operating system:
https://github.com/coder/coder/releases
Run `+pretty.Sprint(cliui.DefaultStyles.Code, "coder login "+client.URL.String())+` to authenticate.
Your email is: `+pretty.Sprint(cliui.DefaultStyles.Field, email)+`
`+authenticationMethod+`
Create a workspace  `+pretty.Sprint(cliui.DefaultStyles.Code, "coder create")+`!`)
			return nil
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:          "email",
			FlagShorthand: "e",
			Description:   "Specifies an email address for the new user.",
			Value:         serpent.StringOf(&email),
		},
		{
			Flag:          "username",
			FlagShorthand: "u",
			Description:   "Specifies a username for the new user.",
			Value: serpent.Validate(serpent.StringOf(&username), func(_username *serpent.String) error {
				username := _username.String()
				if username != "" {
					err := codersdk.NameValid(username)
					if err != nil {
						return fmt.Errorf("username %q is invalid: %w", username, err)
					}
				}
				return nil
			}),
		},
		{
			Flag:          "full-name",
			FlagShorthand: "n",
			Description:   "Specifies an optional human-readable name for the new user.",
			Value:         serpent.StringOf(&name),
		},
		{
			Flag:          "password",
			FlagShorthand: "p",
			Description:   "Specifies a password for the new user.",
			Value:         serpent.StringOf(&password),
		},
		{
			Flag:   "disable-login",
			Hidden: true,
			Description: "Deprecated: Use '--login-type=none'. \nDisabling login for a user prevents the user from authenticating via password or IdP login. Authentication requires an API key/token generated by an admin. " +
				"Be careful when using this flag as it can lock the user out of their account.",
			Value: serpent.BoolOf(&disableLogin),
		},
		{
			Flag: "login-type",
			Description: fmt.Sprintf("Optionally specify the login type for the user. Valid values are: %s. "+
				"Using 'none' prevents the user from authenticating and requires an API key/token to be generated by an admin.",
				strings.Join([]string{
					string(codersdk.LoginTypePassword), string(codersdk.LoginTypeNone), string(codersdk.LoginTypeGithub), string(codersdk.LoginTypeOIDC),
				}, ", ",
				)),
			Value: serpent.StringOf(&loginType),
		},
	}
	orgContext.AttachOptions(cmd)
	return cmd
}
