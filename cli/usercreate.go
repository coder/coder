package cli

import (
	"fmt"

	"github.com/go-playground/validator/v10"
	"golang.org/x/xerrors"

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
	"github.com/coder/coder/cryptorand"
)

func (r *RootCmd) userCreate() *clibase.Cmd {
	var (
		email    string
		username string
		password string
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use: "create",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			organization, err := CurrentOrganization(inv, client)
			if err != nil {
				return err
			}
			if username == "" {
				username, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text: "Username:",
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
				password, err = cryptorand.StringCharset(cryptorand.Human, 20)
				if err != nil {
					return err
				}
			}

			_, err = client.CreateUser(inv.Context(), codersdk.CreateUserRequest{
				Email:          email,
				Username:       username,
				Password:       password,
				OrganizationID: organization.ID,
			})
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintln(inv.Stderr, `A new user has been created!
Share the instructions below to get them started.
`+cliui.DefaultStyles.Placeholder.Render("—————————————————————————————————————————————————")+`
Download the Coder command line for your operating system:
https://github.com/coder/coder/releases

Run `+cliui.DefaultStyles.Code.Render("coder login "+client.URL.String())+` to authenticate.

Your email is: `+cliui.DefaultStyles.Field.Render(email)+`
Your password is: `+cliui.DefaultStyles.Field.Render(password)+`

Create a workspace  `+cliui.DefaultStyles.Code.Render("coder create")+`!`)
			return nil
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:          "email",
			FlagShorthand: "e",
			Description:   "Specifies an email address for the new user.",
			Value:         clibase.StringOf(&email),
		},
		{
			Flag:          "username",
			FlagShorthand: "u",
			Description:   "Specifies a username for the new user.",
			Value:         clibase.StringOf(&username),
		},
		{
			Flag:          "password",
			FlagShorthand: "p",
			Description:   "Specifies a password for the new user.",
			Value:         clibase.StringOf(&password),
		},
	}
	return cmd
}
