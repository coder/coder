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

	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
	"github.com/coder/coder/codersdk"
)

var jwtRegexp = regexp.MustCompile(`^[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+\.[A-Za-z0-9_-]+$`)

func (r *RootCmd) licenses() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Short:   "Add, delete, and list licenses",
		Use:     "licenses",
		Aliases: []string{"license"},
		Handler: func(inv *clibase.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*clibase.Cmd{
			r.licenseAdd(),
			r.licensesList(),
			r.licenseDelete(),
		},
	}
	return cmd
}

func (r *RootCmd) licenseAdd() *clibase.Cmd {
	var (
		filename string
		license  string
		debug    bool
	)
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:   "add [-f file | -l license]",
		Short: "Add license to Coder deployment",
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			var err error
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
			_, _ = fmt.Fprintf(inv.Stdout, "License with ID %d added\n", licResp.ID)
			return nil
		},
	}
	cmd.Options = clibase.OptionSet{
		{
			Flag:          "file",
			FlagShorthand: "f",
			Description:   "Load license from file.",
			Value:         clibase.StringOf(&filename),
		},
		{
			Flag:          "license",
			FlagShorthand: "l",
			Description:   "License string.",
			Value:         clibase.StringOf(&license),
		},
		{
			Flag:        "debug",
			Description: "Output license claims for debugging.",
			Value:       clibase.BoolOf(&debug),
		},
	}
	return cmd
}

func validJWT(s string) error {
	if jwtRegexp.MatchString(s) {
		return nil
	}
	return xerrors.New("Invalid license")
}

func (r *RootCmd) licensesList() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:     "list",
		Short:   "List licenses (including expired)",
		Aliases: []string{"ls"},
		Middleware: clibase.Chain(
			clibase.RequireNArgs(0),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			licenses, err := client.Licenses(inv.Context())
			if err != nil {
				return err
			}
			// Ensure that we print "[]" instead of "null" when there are no licenses.
			if licenses == nil {
				licenses = make([]codersdk.License, 0)
			}

			for i, license := range licenses {
				newClaims, err := convertLicenseExpireTime(license.Claims)
				if err != nil {
					return err
				}
				licenses[i].Claims = newClaims
			}

			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(licenses)
		},
	}
	return cmd
}

func (r *RootCmd) licenseDelete() *clibase.Cmd {
	client := new(codersdk.Client)
	cmd := &clibase.Cmd{
		Use:     "delete <id>",
		Short:   "Delete license by ID",
		Aliases: []string{"del"},
		Middleware: clibase.Chain(
			clibase.RequireNArgs(1),
			r.InitClient(client),
		),
		Handler: func(inv *clibase.Invocation) error {
			id, err := strconv.ParseInt(inv.Args[0], 10, 32)
			if err != nil {
				return xerrors.Errorf("license ID must be an integer: %s", inv.Args[0])
			}
			err = client.DeleteLicense(inv.Context(), int32(id))
			if err != nil {
				return err
			}
			_, _ = fmt.Fprintf(inv.Stdout, "License with ID %d deleted\n", id)
			return nil
		},
	}
	return cmd
}

func convertLicenseExpireTime(licenseClaims map[string]interface{}) (map[string]interface{}, error) {
	if licenseClaims["license_expires"] != nil {
		licenseExpiresNumber, ok := licenseClaims["license_expires"].(json.Number)
		if !ok {
			return licenseClaims, xerrors.Errorf("could not convert license_expires to json.Number")
		}

		licenseExpires, err := licenseExpiresNumber.Int64()
		if err != nil {
			return licenseClaims, xerrors.Errorf("could not convert license_expires to int64: %w", err)
		}

		t := time.Unix(licenseExpires, 0)
		rfc3339Format := t.Format(time.RFC3339)

		claimsCopy := make(map[string]interface{}, len(licenseClaims))
		for k, v := range licenseClaims {
			claimsCopy[k] = v
		}

		claimsCopy["license_expires"] = rfc3339Format
		return claimsCopy, nil
	}
	return licenseClaims, nil
}
