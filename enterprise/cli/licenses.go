package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/xerrors"
	"gvisor.dev/gvisor/runsc/cmd"

	agpl "github.com/coder/coder/cli"
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
		Handler: func(inv *clibase.Invokation) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		licenseAdd(),
		licensesList(),
		licenseDelete(),
	)
	return cmd
}

func (r *RootCmd) licenseAdd() *clibase.Cmd {
	var (
		filename string
		license  string
		debug    bool
	)
	cmd := &clibase.Cmd{
		Use:        "add [-f file | -l license]",
		Short:      "Add license to Coder deployment",
		Middleware: clibase.RequireNArgs(0),
		Handler: func(inv *clibase.Invokation) error {
			client, err := agpl.CreateClient(cmd)
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
			_, _ = fmt.Fprintf(inv.Stdout, "License with ID %d added\n", licResp.ID)
			return nil
		},
	}
	cmd.Flags().StringVarP(&filename, "file", "f", "", "Load license from file")
	cmd.Flags().StringVarP(&license, "license", "l", "", "License string")
	cmd.Flags().BoolVar(&debug, "debug", false, "Output license claims for debugging")
	return cmd
}

func validJWT(s string) error {
	if jwtRegexp.MatchString(s) {
		return nil
	}
	return xerrors.New("Invalid license")
}

func (r *RootCmd) licensesList() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:        "list",
		Short:      "List licenses (including expired)",
		Aliases:    []string{"ls"},
		Middleware: clibase.RequireNArgs(0),
		Handler: func(inv *clibase.Invokation) error {
			client, err := agpl.CreateClient(cmd)
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

			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(licenses)
		},
	}
	return cmd
}

func (r *RootCmd) licenseDelete() *clibase.Cmd {
	cmd := &clibase.Cmd{
		Use:        "delete <id>",
		Short:      "Delete license by ID",
		Aliases:    []string{"del", "rm"},
		Middleware: clibase.RequireNArgs(1),
		Handler: func(inv *clibase.Invokation) error {
			client, err := agpl.CreateClient(cmd)
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
			_, _ = fmt.Fprintf(inv.Stdout, "License with ID %d deleted\n", id)
			return nil
		},
	}
	return cmd
}
