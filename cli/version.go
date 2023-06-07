package cli

import (
	"fmt"
	"strings"
	"time"

	"github.com/coder/coder/buildinfo"
	"github.com/coder/coder/cli/clibase"
	"github.com/coder/coder/cli/cliui"
)

// versionInfo wraps the stuff we get from buildinfo so that it's
// easier to emit in different formats.
type versionInfo struct {
	Version     string    `json:"version"`
	BuildTime   time.Time `json:"build_time"`
	ExternalURL string    `json:"external_url"`
	Slim        bool      `json:"slim"`
	AGPL        bool      `json:"agpl"`
}

// String() implements Stringer
func (vi versionInfo) String() string {
	var str strings.Builder
	_, _ = str.WriteString("Coder ")
	if vi.AGPL {
		_, _ = str.WriteString("(AGPL) ")
	}
	_, _ = str.WriteString(vi.Version)

	if !vi.BuildTime.IsZero() {
		_, _ = str.WriteString(" " + vi.BuildTime.Format(time.UnixDate))
	}
	_, _ = str.WriteString("\r\n" + vi.ExternalURL + "\r\n\r\n")

	if vi.Slim {
		_, _ = str.WriteString(fmt.Sprintf("Slim build of Coder, does not support the %s subcommand.", cliui.DefaultStyles.Code.Render("server")))
	} else {
		_, _ = str.WriteString(fmt.Sprintf("Full build of Coder, supports the %s subcommand.", cliui.DefaultStyles.Code.Render("server")))
	}
	return str.String()
}

func defaultVersionInfo() *versionInfo {
	buildTime, _ := buildinfo.Time()
	return &versionInfo{
		Version:     buildinfo.Version(),
		BuildTime:   buildTime,
		ExternalURL: buildinfo.ExternalURL(),
		Slim:        buildinfo.IsSlim(),
		AGPL:        buildinfo.IsAGPL(),
	}
}

// version prints the coder version
func (*RootCmd) version(versionInfo func() *versionInfo) *clibase.Cmd {
	var (
		formatter = cliui.NewOutputFormatter(
			cliui.TextFormat(),
			cliui.JSONFormat(),
		)
		vi = versionInfo()
	)

	cmd := &clibase.Cmd{
		Use:     "version",
		Short:   "Show coder version",
		Options: clibase.OptionSet{},
		Handler: func(inv *clibase.Invocation) error {
			out, err := formatter.Format(inv.Context(), vi)
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
