package reconnectingpty

import (
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/httpapi"
	"github.com/coder/coder/v2/codersdk/workspacesdk"
)

const (
	DefaultWidth   = 80
	DefaultHeight  = 24
	DefaultTimeout = httpapi.Duration(5 * time.Minute)
)

type Config struct {
	// AgentID is the ID of the agent to run the command in.
	AgentID uuid.UUID `json:"agent_id"`
	// Init is the initial packet to send to the agent when launching the TTY.
	// If the ID is not set, defaults to a random UUID. If the width or height
	// is not set, defaults to 80x24. If the command is not set, defaults to
	// opening a login shell. Command runs in the default shell.
	Init workspacesdk.AgentReconnectingPTYInit `json:"init"`
	// Timeout is the duration to wait for the command to exit. Defaults to
	// 5 minutes.
	Timeout httpapi.Duration `json:"timeout"`
	// ExpectTimeout means we expect the timeout to be reached (i.e. the command
	// doesn't exit within the given timeout).
	ExpectTimeout bool `json:"expect_timeout"`
	// ExpectOutput checks that the given string is present in the output. The
	// string must be present on a single line.
	ExpectOutput string `json:"expect_output"`
	// LogOutput determines whether the output of the command should be logged.
	// For commands that produce a lot of output this should be disabled to
	// avoid loadtest OOMs. All log output is still read and discarded if this
	// is false.
	LogOutput bool `json:"log_output"`
}

func (c Config) Validate() error {
	if c.AgentID == uuid.Nil {
		return xerrors.New("agent_id must be set")
	}
	if c.Timeout < 0 {
		return xerrors.New("timeout must be a positive value")
	}

	return nil
}
