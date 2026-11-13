package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

func (r *RootCmd) identities() *serpent.Command {
	cmd := &serpent.Command{
		Use:   "identities",
		Short: "Manage delegated identities (experimental)",
		Long: "A delegated identity is an identity for something that acts under you, for example a coding agent. " +
			"It owns the workspaces created on its behalf, is named in audit logs, and can never do more than you.\n" +
			FormatExamples(
				Example{
					Description: "Create an identity that may only read its own workspaces",
					Command:     "coder identities create my-agent --scope workspace:read",
				},
				Example{
					Description: "Create a workspace on its behalf",
					Command:     "coder create my-agent/task-1 --template docker",
				},
				Example{
					Description: "Change what it may do",
					Command:     "coder identities update my-agent --scope workspace:read --scope template:read",
				},
			),
		Aliases: []string{"identity"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.createIdentity(),
			r.listIdentities(),
			r.showIdentity(),
			r.updateIdentity(),
			r.deleteIdentity(),
		},
	}
	return cmd
}

type identityRow struct {
	codersdk.DelegatedIdentity `table:"-"`

	Name      string    `json:"-" table:"name,default_sort"`
	Parent    string    `json:"-" table:"parent"`
	Root      string    `json:"-" table:"root user"`
	Scopes    string    `json:"-" table:"scopes"`
	Effective string    `json:"-" table:"effective scopes"`
	Created   time.Time `json:"-" table:"created at"`
	ID        string    `json:"-" table:"id"`
}

func identityScopes(scopes []codersdk.APIKeyScope) string {
	if len(scopes) == 0 {
		return "(none)"
	}
	names := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		names = append(names, string(scope))
	}
	return strings.Join(names, ", ")
}

func identityRows(identities []codersdk.DelegatedIdentity) []identityRow {
	rows := make([]identityRow, 0, len(identities))
	for _, identity := range identities {
		rows = append(rows, identityRow{
			DelegatedIdentity: identity,
			Name:              identity.Name,
			Parent:            identity.ParentName,
			Root:              identity.RootUsername,
			Scopes:            identityScopes(identity.Scopes),
			Effective:         identityScopes(identity.EffectiveScopes),
			Created:           identity.CreatedAt,
			ID:                identity.ID.String(),
		})
	}
	return rows
}

func identityFormatter() *cliui.OutputFormatter {
	return cliui.NewOutputFormatter(
		cliui.TableFormat([]identityRow{}, []string{"name", "parent", "root user", "scopes", "effective scopes"}),
		cliui.JSONFormat(),
	)
}

func identityScopeOption(scopes *[]string) serpent.Option {
	return serpent.Option{
		Flag:        "scope",
		Description: "A scope the identity may use with the Coder API, as API tokens name them. Repeat for more. Without any, the identity can do nothing.",
		Value:       serpent.StringArrayOf(scopes),
	}
}

func identityAPIKeyScopes(scopes []string) []codersdk.APIKeyScope {
	out := make([]codersdk.APIKeyScope, 0, len(scopes))
	for _, scope := range scopes {
		out = append(out, codersdk.APIKeyScope(scope))
	}
	return out
}

func (r *RootCmd) createIdentity() *serpent.Command {
	var (
		scopes []string
		parent string
		org    string
	)
	formatter := identityFormatter()
	cmd := &serpent.Command{
		Use:   "create [name]",
		Short: "Create a delegated identity under you",
		Long: "Creating an identity returns a record and no credential. " +
			"The identity gets its credentials where it is used: in a workspace created on its behalf.",
		Middleware: serpent.Chain(
			serpent.RequireRangeArgs(0, 1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			req := codersdk.CreateDelegatedIdentityRequest{
				Scopes: identityAPIKeyScopes(scopes),
			}
			if len(inv.Args) == 1 {
				req.Name = inv.Args[0]
			}
			if parent != "" {
				// The parent is an identity below the caller, or with
				// permission across the organization any member.
				id, err := uuid.Parse(parent)
				if err != nil {
					identity, identityErr := client.DelegatedIdentity(inv.Context(), parent)
					if identityErr != nil {
						user, userErr := client.User(inv.Context(), parent)
						if userErr != nil {
							return xerrors.Errorf("find the parent %q: %w", parent, identityErr)
						}
						id = user.ID
					} else {
						id = identity.ID
					}
				}
				req.ParentID = id
			}
			if org != "" {
				organization, err := client.OrganizationByName(inv.Context(), org)
				if err != nil {
					return xerrors.Errorf("find the organization %q: %w", org, err)
				}
				req.OrganizationID = organization.ID
			}
			identity, err := client.CreateDelegatedIdentity(inv.Context(), req)
			if err != nil {
				return err
			}
			out, err := formatter.Format(inv.Context(), identityRows([]codersdk.DelegatedIdentity{identity}))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	cmd.Options = serpent.OptionSet{
		identityScopeOption(&scopes),
		{
			Flag:        "parent",
			Description: "The delegated identity or user to create the identity under. It defaults to you.",
			Value:       serpent.StringOf(&parent),
		},
		{
			Flag:        "org",
			Description: "The organization of the identity, when its parent is in more than one.",
			Value:       serpent.StringOf(&org),
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) listIdentities() *serpent.Command {
	formatter := identityFormatter()
	cmd := &serpent.Command{
		Use:     "list",
		Aliases: []string{"ls"},
		Short:   "List the delegated identities below you",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			identities, err := client.DelegatedIdentities(inv.Context())
			if err != nil {
				return err
			}
			out, err := formatter.Format(inv.Context(), identityRows(identities))
			if err != nil {
				return err
			}
			if out == "" {
				cliui.Info(inv.Stderr, "No delegated identities found.")
				return nil
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) showIdentity() *serpent.Command {
	formatter := identityFormatter()
	cmd := &serpent.Command{
		Use:   "show <name|id|me>",
		Short: "Show a delegated identity",
		Long:  "With \"me\", show the delegated identity the command itself runs as.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			identity, err := client.DelegatedIdentity(inv.Context(), inv.Args[0])
			if err != nil {
				return err
			}
			out, err := formatter.Format(inv.Context(), identityRows([]codersdk.DelegatedIdentity{identity}))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) updateIdentity() *serpent.Command {
	var scopes []string
	formatter := identityFormatter()
	cmd := &serpent.Command{
		Use:   "update <name|id>",
		Short: "Replace what a delegated identity may do",
		Long:  "The change applies from the identity's next request, to the credentials it already has.",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			identity, err := client.UpdateDelegatedIdentity(inv.Context(), inv.Args[0], codersdk.UpdateDelegatedIdentityRequest{
				Scopes: identityAPIKeyScopes(scopes),
			})
			if err != nil {
				return err
			}
			out, err := formatter.Format(inv.Context(), identityRows([]codersdk.DelegatedIdentity{identity}))
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	cmd.Options = serpent.OptionSet{
		identityScopeOption(&scopes),
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func (r *RootCmd) deleteIdentity() *serpent.Command {
	cmd := &serpent.Command{
		Use:     "delete <name|id>",
		Aliases: []string{"rm"},
		Short:   "Delete a delegated identity that owns no workspaces",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}
			if err := client.DeleteDelegatedIdentity(inv.Context(), inv.Args[0]); err != nil {
				return err
			}
			_, err = fmt.Fprintf(inv.Stdout, "Deleted the delegated identity %s.\n", inv.Args[0])
			return err
		},
	}
	return cmd
}
