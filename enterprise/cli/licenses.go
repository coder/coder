package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/serpent"
)

var jwtRegexp = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)

func (r *RootCmd) licenses() *serpent.Command {
	cmd := &serpent.Command{
		Short:   "Add, delete, and list licenses",
		Use:     "licenses",
		Aliases: []string{"license"},
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			r.licenseAdd(),
			r.licensesList(),
			r.licenseDelete(),
		},
	}
	return cmd
}

func (r *RootCmd) licenseAdd() *serpent.Command {
	var (
		filename  string
		license   string
		debug     bool
		formatter = cliui.NewOutputFormatter(
			cliui.ChangeFormatterData(cliui.TextFormat(), func(data any) (any, error) {
				typed, ok := data.(codersdk.License)
				if !ok {
					return "", xerrors.Errorf("expected License, got %T", data)
				}
				return fmt.Sprintf("License with ID %d added", typed.ID), nil
			}),
			cliui.ChangeFormatterData(cliui.JSONFormat(), func(data any) (any, error) {
				typed, ok := data.(codersdk.License)
				if !ok {
					return nil, xerrors.Errorf("expected License, got %T", data)
				}
				// Match the enrichment done by `licenses list --output=json`.
				if humanExp, err := typed.ExpiresAt(); err == nil {
					typed.Claims[codersdk.LicenseExpiryClaim+"_human"] = humanExp.Format(time.RFC3339)
				}
				return typed, nil
			}),
		)
	)
	cmd := &serpent.Command{
		Use:   "add [-f file | -l license]",
		Short: "Add license to Coder deployment",
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			switch {
			case filename != "" && license != "":
				return xerrors.New("only one of (--file, --license) may be specified")

			case filename == "" && license == "":
				license, err = cliui.Prompt(inv, cliui.PromptOptions{
					Text:     "Paste license:",
					Secret:   true,
					Validate: validJWT,
				})
				if err != nil {
					return err
				}

			case filename != "" && license == "":
				var r io.Reader
				if filename == "-" {
					r = inv.Stdin
				} else {
					f, err := os.Open(filename)
					if err != nil {
						return err
					}
					defer f.Close()
					r = f
				}
				lb, err := io.ReadAll(r)
				if err != nil {
					return err
				}
				license = string(lb)
			}
			license = strings.Trim(license, " \n")
			err = validJWT(license)
			if err != nil {
				return err
			}

			licResp, err := client.AddLicense(
				inv.Context(),
				codersdk.AddLicenseRequest{License: license},
			)
			if err != nil {
				return err
			}
			if debug {
				enc := json.NewEncoder(inv.Stdout)
				enc.SetIndent("", "  ")
				return enc.Encode(licResp)
			}

			out, err := formatter.Format(inv.Context(), licResp)
			if err != nil {
				return err
			}
			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	cmd.Options = serpent.OptionSet{
		{
			Flag:          "file",
			FlagShorthand: "f",
			Description:   "Load license from file.",
			Value:         serpent.StringOf(&filename),
		},
		{
			Flag:          "license",
			FlagShorthand: "l",
			Description:   "License string.",
			Value:         serpent.StringOf(&license),
		},
		{
			Flag:        "debug",
			Description: "Output license claims for debugging.",
			Value:       serpent.BoolOf(&debug),
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

func validJWT(s string) error {
	if jwtRegexp.MatchString(s) {
		return nil
	}
	return xerrors.New("Invalid license")
}

func (r *RootCmd) licensesList() *serpent.Command {
	formatter := cliutil.NewLicenseFormatter()
	cmd := &serpent.Command{
		Use:     "list",
		Short:   "List licenses (including expired)",
		Aliases: []string{"ls"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			licenses, err := client.Licenses(inv.Context())
			if err != nil {
				return err
			}
			// Ensure that we print "[]" instead of "null" when there are no licenses.
			if licenses == nil {
				licenses = make([]codersdk.License, 0)
			}

			out, err := formatter.Format(inv.Context(), licenses)
			if err != nil {
				return err
			}

			if out == "" {
				cliui.Infof(inv.Stderr, "No licenses found.")
				return nil
			}

			_, err = fmt.Fprintln(inv.Stdout, out)
			return err
		},
	}
	formatter.AttachOptions(&cmd.Options)
	return cmd
}

type licenseDeleteResponse struct {
	ID int32 `json:"id"`
}

func (r *RootCmd) licenseDelete() *serpent.Command {
	formatter := cliui.NewOutputFormatter(
		cliui.ChangeFormatterData(cliui.TextFormat(), func(data any) (any, error) {
			typed, ok := data.(licenseDeleteResponse)
			if !ok {
				return "", xerrors.Errorf("expected licenseDeleteResponse, got %T", data)
			}
			return fmt.Sprintf("License with ID %d deleted", typed.ID), nil
		}),
		cliui.JSONFormat(),
	)
	cmd := &serpent.Command{
		Use:     "delete <id>",
		Short:   "Delete license by ID",
		Aliases: []string{"del"},
		Middleware: serpent.Chain(
			serpent.RequireNArgs(1),
		),
		Handler: func(inv *serpent.Invocation) error {
			client, err := r.InitClient(inv)
			if err != nil {
				return err
			}

			id, err := strconv.ParseInt(inv.Args[0], 10, 32)
			if err != nil {
				return xerrors.Errorf("license ID must be an integer: %s", inv.Args[0])
			}
			err = client.DeleteLicense(inv.Context(), int32(id))
			if err != nil {
				return err
			}

			out, err := formatter.Format(inv.Context(), licenseDeleteResponse{ID: int32(id)})
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
