package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/x/agentdesktop/desktopruntime"
	"github.com/coder/coder/v2/agent/x/agentdesktop/xvnc"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/serpent"
)

// agentDesktop groups the built-in virtual desktop commands under
// `coder agent desktop`. The desktop runtime (a static Xvnc server) is
// embedded in the agent binary; nothing is downloaded or installed inside the
// workspace.
func agentDesktop() *serpent.Command {
	return &serpent.Command{
		Use:   "desktop",
		Short: "Manage the built-in virtual desktop (experimental).",
		Long: "The virtual desktop runs an Xvnc server embedded in the Coder binary. " +
			"It listens on loopback only and relies on the workspace agent to broker access.",
		Handler: func(inv *serpent.Invocation) error {
			return inv.Command.HelpHandler(inv)
		},
		Children: []*serpent.Command{
			agentDesktopRuntime(),
			agentDesktopUp(),
		},
	}
}

// agentDesktopRuntimeInfo is the JSON output of `coder agent desktop runtime`.
type agentDesktopRuntimeInfo struct {
	Available  bool   `json:"available"`
	Archive    string `json:"archive,omitempty"`
	SHA256     string `json:"sha256,omitempty"`
	SizeBytes  int    `json:"size_bytes"`
	CacheDir   string `json:"cache_dir"`
	RuntimeDir string `json:"runtime_dir,omitempty"`
	Unpacked   bool   `json:"unpacked"`
}

func agentDesktopRuntime() *serpent.Command {
	var (
		cacheDir string
		unpack   bool
	)
	cmd := &serpent.Command{
		Use:   "runtime",
		Short: "Show the embedded desktop runtime and optionally unpack it.",
		Handler: func(inv *serpent.Invocation) error {
			var err error
			if cacheDir == "" {
				cacheDir, err = desktopruntime.DefaultCacheDir()
				if err != nil {
					return err
				}
			}
			info := agentDesktopRuntimeInfo{
				Available: desktopruntime.Available(),
				Archive:   desktopruntime.ArchiveName,
				SHA256:    desktopruntime.SHA256(),
				SizeBytes: len(desktopruntime.Archive()),
				CacheDir:  cacheDir,
			}
			if info.Available {
				info.RuntimeDir = desktopruntime.RuntimeDir(cacheDir)
				if unpack {
					if _, err := desktopruntime.Unpack(inv.Context(), cacheDir); err != nil {
						return err
					}
				}
				info.Unpacked = desktopruntime.Validate(info.RuntimeDir) == nil
			} else if unpack {
				return desktopruntime.ErrNotAvailable
			}
			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")
			return enc.Encode(info)
		},
	}
	cmd.Options = serpent.OptionSet{
		agentDesktopCacheDirOption(&cacheDir),
		{
			Flag:        "unpack",
			Description: "Unpack the runtime into the cache directory if it is not already present.",
			Value:       serpent.BoolOf(&unpack),
		},
	}
	return cmd
}

// agentDesktopSession is the JSON output of `coder agent desktop up`.
type agentDesktopSession struct {
	Display    int    `json:"display"`
	DisplayEnv string `json:"display_env"`
	VNCAddr    string `json:"vnc_addr"`
	VNCPort    int    `json:"vnc_port"`
	PID        int    `json:"pid"`
	RuntimeDir string `json:"runtime_dir"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
}

func agentDesktopUp() *serpent.Command {
	var (
		cacheDir string
		display  int64
		width    int64
		height   int64
		logPath  string
	)
	cmd := &serpent.Command{
		Use:   "up",
		Short: "Start an Xvnc server from the embedded runtime and block until it exits.",
		Long: "Prints session details as JSON once the VNC server accepts connections. " +
			"The server is stopped when this command receives an interrupt.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()
			if !desktopruntime.Available() {
				return desktopruntime.ErrNotAvailable
			}
			var err error
			if cacheDir == "" {
				cacheDir, err = desktopruntime.DefaultCacheDir()
				if err != nil {
					return err
				}
			}
			runtimeDir, err := desktopruntime.Unpack(ctx, cacheDir)
			if err != nil {
				return xerrors.Errorf("unpack desktop runtime: %w", err)
			}

			var logFile *os.File
			if logPath != "" {
				logFile, err = os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
				if err != nil {
					return xerrors.Errorf("open log file: %w", err)
				}
				defer logFile.Close()
			}

			srv, err := xvnc.Start(ctx, xvnc.Options{
				RuntimeDir: runtimeDir,
				Display:    int(display),
				Width:      int(width),
				Height:     int(height),
				Log:        logFile,
			})
			if err != nil {
				return xerrors.Errorf("start Xvnc: %w", err)
			}

			enc := json.NewEncoder(inv.Stdout)
			enc.SetIndent("", "  ")
			if err := enc.Encode(agentDesktopSession{
				Display:    srv.Display,
				DisplayEnv: fmt.Sprintf(":%d", srv.Display),
				VNCAddr:    srv.Addr(),
				VNCPort:    srv.Port,
				PID:        srv.Cmd.Process.Pid,
				RuntimeDir: runtimeDir,
				Width:      int(width),
				Height:     int(height),
			}); err != nil {
				_ = srv.Stop(5 * time.Second)
				return err
			}

			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, InterruptSignals...)
			defer signal.Stop(sigs)
			select {
			case <-ctx.Done():
			case <-sigs:
				cliui.Infof(inv.Stderr, "Stopping Xvnc")
			case <-srv.Done():
				if err := srv.Err(); err != nil {
					return xerrors.Errorf("Xvnc exited: %w", err)
				}
				return nil
			}
			return srv.Stop(5 * time.Second)
		},
	}
	cmd.Options = serpent.OptionSet{
		agentDesktopCacheDirOption(&cacheDir),
		{
			Flag:        "display",
			Description: "X display number to use. Defaults to the first free display at or above 10.",
			Value:       serpent.Int64Of(&display),
		},
		{
			Flag:        "width",
			Description: "Initial framebuffer width in pixels.",
			Default:     "1280",
			Value:       serpent.Int64Of(&width),
		},
		{
			Flag:        "height",
			Description: "Initial framebuffer height in pixels.",
			Default:     "800",
			Value:       serpent.Int64Of(&height),
		},
		{
			Flag:        "log-file",
			Description: "Append Xvnc output to this file instead of discarding it.",
			Value:       serpent.StringOf(&logPath),
		},
	}
	return cmd
}

func agentDesktopCacheDirOption(dst *string) serpent.Option {
	return serpent.Option{
		Flag:        "cache-dir",
		Env:         "CODER_AGENT_DESKTOP_CACHE_DIR",
		Description: "Directory the runtime is unpacked into. Defaults to $XDG_CACHE_HOME/coder or ~/.cache/coder.",
		Value:       serpent.StringOf(dst),
	}
}
