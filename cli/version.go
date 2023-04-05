package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
)

// version prints the coder version
func (*RootCmd) version() *clibase.Cmd {
	handleHuman := func(inv *clibase.Invocation) error {
		var str strings.Builder
		_, _ = str.WriteString("Coder ")
		if buildinfo.IsAGPL() {
			_, _ = str.WriteString("(AGPL) ")
		}
		_, _ = str.WriteString(buildinfo.Version())
		buildTime, valid := buildinfo.Time()
		if valid {
			_, _ = str.WriteString(" " + buildTime.Format(time.UnixDate))
		}
		_, _ = str.WriteString("\r\n" + buildinfo.ExternalURL() + "\r\n\r\n")

		if buildinfo.IsSlim() {
			_, _ = str.WriteString(fmt.Sprintf("Slim build of Coder, does not support the %s subcommand.\n", cliui.Styles.Code.Render("server")))
		} else {
			_, _ = str.WriteString(fmt.Sprintf("Full build of Coder, supports the %s subcommand.\n", cliui.Styles.Code.Render("server")))
		}

		_, _ = fmt.Fprint(inv.Stdout, str.String())
		return nil
	}

	handleJSON := func(inv *clibase.Invocation) error {
		buildTime, _ := buildinfo.Time()
		versionInfo := struct {
			Version     string `json:"version"`
			BuildTime   string `json:"build_time"`
			ExternalURL string `json:"external_url"`
			Slim        bool   `json:"slim"`
			AGPL        bool   `json:"agpl"`
		}{
			Version:     buildinfo.Version(),
			BuildTime:   buildTime.Format(time.UnixDate),
			ExternalURL: buildinfo.ExternalURL(),
			Slim:        buildinfo.IsSlim(),
			AGPL:        buildinfo.IsAGPL(),
		}

		enc := json.NewEncoder(inv.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(versionInfo)
	}

	var outputJSON bool

	return &clibase.Cmd{
		Use:   "version",
		Short: "Show coder version",
		Options: clibase.OptionSet{
			{
				Flag:        "json",
				Description: "Emit version information in machine-readable JSON format.",
				Value:       clibase.BoolOf(&outputJSON),
			},
		},
		Handler: func(inv *clibase.Invocation) error {
			if outputJSON {
				return handleJSON(inv)
			}
			return handleHuman(inv)
		},
	}
}
