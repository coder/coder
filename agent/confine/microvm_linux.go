//go:build linux

package confine

import (
	"context"
	"errors"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coder-sandbox/hostvm"
	sandboxproxy "github.com/coder/coder/coder-sandbox/proxy"
)

const embeddedMicroVMCleanupTimeout = time.Minute

const (
	embeddedAgentBootstrapPath = "/var/lib/coder/bootstrap.sh"
	embeddedAgentProfilePath   = "/etc/profile.d/coder-sandbox.sh"
)

type embeddedMicroVMConfig struct {
	hostOptions  hostvm.Options
	agentCommand string
	evaluator    *policyEvaluator
	recorder     *sandboxEventRecorder
}

// StartEmbeddedMicroVM boots a microVM, installs the live AI egress evaluator
// on its gateway proxy, and starts the child Coder agent inside the guest.
func StartEmbeddedMicroVM(ctx context.Context, options MicroVMOptions) (*EmbeddedSandbox, error) {
	config, err := newEmbeddedMicroVMConfig(options)
	if err != nil {
		return nil, err
	}
	reportMicroVMProgress(options.Progress, "provisioning microVM runtime and guest image")
	vm, err := hostvm.Boot(ctx, config.hostOptions)
	if err != nil {
		return nil, xerrors.Errorf("boot embedded microVM: %w", err)
	}
	reportMicroVMProgress(options.Progress, "microVM booted")
	reportMicroVMProgress(options.Progress, "launching guest agent")
	status, execErr := vm.Exec(ctx, config.agentCommand)
	if execErr == nil && status == 0 {
		reportMicroVMProgress(options.Progress, "guest agent launched")
		return &EmbeddedSandbox{vm: vm}, nil
	}

	cleanupCtx, cancel := context.WithTimeout(context.Background(), embeddedMicroVMCleanupTimeout)
	defer cancel()
	cleanupErr := vm.Close(cleanupCtx)
	if execErr != nil {
		return nil, errors.Join(xerrors.Errorf("start embedded microVM agent: %w", execErr), cleanupErr)
	}
	return nil, errors.Join(xerrors.Errorf("start embedded microVM agent: guest command exited with status %d", status), cleanupErr)
}

func newEmbeddedMicroVMConfig(options MicroVMOptions) (embeddedMicroVMConfig, error) {
	binaryPath, agentURL, err := validateMicroVMOptions(options)
	if err != nil {
		return embeddedMicroVMConfig{}, err
	}

	destination, err := ControlChannelDestinationOptions(agentURL)
	if err != nil {
		return embeddedMicroVMConfig{}, xerrors.Errorf("configure embedded microVM control channel: %w", err)
	}
	destination.LookupNetIP = options.Destination.LookupNetIP
	evaluator := newPolicyEvaluator(options.Policy, destination)
	recorder := newSandboxEventRecorder(options.Event)
	ca, caPEM, err := ephemeralProxyCA()
	if err != nil {
		return embeddedMicroVMConfig{}, err
	}
	server, err := sandboxproxy.New(ca, caPEM, recorder)
	if err != nil {
		return embeddedMicroVMConfig{}, xerrors.Errorf("create embedded microVM proxy: %w", err)
	}
	subject := &sandboxproxy.Subject{
		ID:     options.Name,
		Name:   options.Name,
		Policy: evaluator,
	}
	mounts := []hostvm.Mount{{
		Source: filepath.Dir(binaryPath), Target: embeddedCoderGuestDir,
		ReadOnly: true, Nosuid: true, Nodev: true,
	}}
	guestCAFile, caBundlePath, err := resolveHostCABundle(options.CABundlePath)
	if err != nil {
		return embeddedMicroVMConfig{}, err
	}
	if caBundlePath != "" {
		mounts = append(mounts, hostvm.Mount{
			Source: filepath.Dir(caBundlePath), Target: embeddedCAGuestDir,
			ReadOnly: true, Noexec: true, Nosuid: true, Nodev: true,
		})
	}
	return embeddedMicroVMConfig{
		hostOptions: hostvm.Options{
			Image:     options.Image,
			Name:      options.Name,
			CacheDir:  options.CacheDir,
			StateDir:  options.StateDir,
			CPUs:      options.CPUs,
			MemoryMiB: options.MemoryMiB,
			Proxy:     server,
			Subject:   subject,
			Mounts:    mounts,
		},
		agentCommand: embeddedAgentCommand(
			embeddedCoderGuestDir+"/"+filepath.Base(binaryPath), guestCAFile,
			agentURL.String(), options.AgentToken, options.SessionToken,
		),
		evaluator: evaluator,
		recorder:  recorder,
	}, nil
}

func validateMicroVMOptions(options MicroVMOptions) (string, *url.URL, error) {
	if !embeddedMicroVMNamePattern.MatchString(options.Name) {
		return "", nil, xerrors.Errorf("invalid embedded microVM name %q", options.Name)
	}
	if strings.TrimSpace(options.Image) == "" {
		return "", nil, xerrors.New("embedded microVM image is required")
	}
	if strings.TrimSpace(options.CacheDir) == "" {
		return "", nil, xerrors.New("embedded microVM cache directory is required")
	}
	if strings.TrimSpace(options.StateDir) == "" {
		return "", nil, xerrors.New("embedded microVM state directory is required")
	}
	if options.CPUs < 0 {
		return "", nil, xerrors.New("embedded microVM CPU count cannot be negative")
	}
	if options.MemoryMiB <= 0 {
		return "", nil, xerrors.New("embedded microVM memory must be positive")
	}
	if options.Policy == nil {
		return "", nil, xerrors.New("embedded microVM policy engine is required")
	}
	if strings.TrimSpace(options.AgentToken) == "" {
		return "", nil, xerrors.New("embedded microVM agent token is required")
	}
	if strings.TrimSpace(options.CoderBinaryPath) == "" {
		return "", nil, xerrors.New("embedded microVM Coder binary path is required")
	}

	agentURL, err := url.Parse(options.AgentURL)
	if err != nil || agentURL.Scheme == "" || agentURL.Hostname() == "" {
		return "", nil, xerrors.Errorf("invalid embedded microVM agent URL %q", options.AgentURL)
	}
	switch agentURL.Scheme {
	case "http", "https":
	default:
		return "", nil, xerrors.Errorf("invalid embedded microVM agent URL scheme %q", agentURL.Scheme)
	}

	binaryPath, err := filepath.Abs(options.CoderBinaryPath)
	if err != nil {
		return "", nil, xerrors.Errorf("resolve Coder binary path: %w", err)
	}
	binaryPath, err = filepath.EvalSymlinks(binaryPath)
	if err != nil {
		return "", nil, xerrors.Errorf("resolve Coder binary symlinks: %w", err)
	}
	info, err := os.Stat(binaryPath)
	if err != nil {
		return "", nil, xerrors.Errorf("stat Coder binary: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", nil, xerrors.Errorf("Coder binary %q is not a regular file", binaryPath)
	}
	return binaryPath, agentURL, nil
}

// resolveHostCABundle returns the guest path for the CA bundle and the host
// bundle path. An explicit override must exist; discovery misses return empty
// values so the guest image's own trust store applies.
func resolveHostCABundle(override string) (guestFile string, hostPath string, err error) {
	if override != "" {
		info, statErr := os.Stat(override)
		if statErr != nil || !info.Mode().IsRegular() {
			return "", "", xerrors.Errorf("CA bundle %q is not a readable file", override)
		}
		return embeddedCAGuestDir + "/" + filepath.Base(override), override, nil
	}
	for _, candidate := range hostCABundlePaths {
		if info, statErr := os.Stat(candidate); statErr == nil && info.Mode().IsRegular() {
			return embeddedCAGuestDir + "/" + filepath.Base(candidate), candidate, nil
		}
	}
	return "", "", nil
}

// caEnvironmentKeys are the reserved guest variables that name a trust store.
var caEnvironmentKeys = []string{
	"CURL_CA_BUNDLE", "GIT_SSL_CAINFO", "NODE_EXTRA_CA_CERTS",
	"REQUESTS_CA_BUNDLE", "SSL_CERT_FILE",
}

func embeddedGuestProxyEnvironment(guestCAFile string) map[string]string {
	// Guest exec sessions do not inherit init's environment, so the
	// reserved proxy and CA variables must be applied explicitly or the
	// agent dials directly and the VM firewall silently drops the traffic.
	// The proxy runs TLS passthrough, so the CA variables must name real
	// roots, not the interception CA, or every TLS verification fails.
	proxyEnv := hostvm.GuestProxyEnv()
	for _, key := range caEnvironmentKeys {
		if guestCAFile == "" {
			delete(proxyEnv, key)
			continue
		}
		proxyEnv[key] = guestCAFile
	}
	return proxyEnv
}

func embeddedAgentBootstrapScript(guestBinaryPath, guestCAFile string) string {
	proxyEnv := embeddedGuestProxyEnvironment(guestCAFile)
	profile := "# Generated by the Coder sandbox bootstrap.\n"
	for _, key := range slices.Sorted(maps.Keys(proxyEnv)) {
		profile += "export " + key + "=" + shellQuote(proxyEnv[key]) + "\n"
	}

	script := `#!/bin/sh
set -eu

waitonexit() {
	status=$?
	if [ "$status" -eq 0 ]; then
		return
	fi
	echo "=== Agent script exited with non-zero code ($status). Sleeping 24h to preserve logs..."
	sleep 86400
}
trap waitonexit EXIT

`
	script += "CODER_BINARY=" + shellQuote(guestBinaryPath) + "\n"
	script += "CODER_BIN_DIR=" + shellQuote(filepath.Dir(guestBinaryPath)) + "\n"
	script += "CODER_BINARY_NAME=" + shellQuote(filepath.Base(guestBinaryPath)) + "\n"
	script += `
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin${PATH:+:$PATH}
export PATH="$CODER_BIN_DIR:$PATH"

ln -sf "$CODER_BINARY" /usr/local/bin/coder 2>/dev/null || true

USER=$(id -un)
HOME=
SHELL=/bin/sh
if command -v getent >/dev/null 2>&1; then
	passwd_entry=$(getent passwd "$USER" 2>/dev/null || true)
	if [ -n "$passwd_entry" ]; then
		HOME=$(printf '%s\n' "$passwd_entry" | cut -d: -f6)
		user_shell=$(printf '%s\n' "$passwd_entry" | cut -d: -f7)
		if [ -n "$user_shell" ]; then
			SHELL=$user_shell
		fi
	fi
fi
if [ -z "$HOME" ]; then
	if [ "$USER" = root ]; then
		HOME=/root
	else
		HOME=/home/$USER
	fi
fi
export USER HOME SHELL
mkdir -p "$HOME"

mkdir -p /etc/profile.d
`
	script += "printf '%s' " + shellQuote(profile) + " >" + shellQuote(embeddedAgentProfilePath) + "\n"
	script += "chmod 0644 " + shellQuote(embeddedAgentProfilePath) + "\n"
	script += `
cd "$CODER_BIN_DIR"
exec "./$CODER_BINARY_NAME" agent
`
	return script
}

func embeddedAgentCommand(guestBinaryPath, guestCAFile, agentURL, agentToken, sessionToken string) string {
	environment := "CODER_AGENT_URL=" + shellQuote(agentURL) +
		" CODER_AGENT_TOKEN=" + shellQuote(agentToken)
	if sessionToken != "" {
		environment += " CODER_SESSION_TOKEN=" + shellQuote(sessionToken)
	}
	proxyEnv := embeddedGuestProxyEnvironment(guestCAFile)
	for _, key := range slices.Sorted(maps.Keys(proxyEnv)) {
		environment += " " + key + "=" + shellQuote(proxyEnv[key])
	}

	script := embeddedAgentBootstrapScript(guestBinaryPath, guestCAFile)
	return "mkdir -p " + shellQuote(filepath.Dir(embeddedAgentBootstrapPath)) +
		" && umask 077" +
		" && printf '%s' " + shellQuote(script) + " >" + shellQuote(embeddedAgentBootstrapPath) +
		" && chmod 0700 " + shellQuote(embeddedAgentBootstrapPath) +
		" && { " + environment + " setsid sh " + shellQuote(embeddedAgentBootstrapPath) +
		" </dev/null >" + shellQuote(embeddedAgentLogPath) + " 2>&1 & }"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
