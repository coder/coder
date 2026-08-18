//go:build linux

package confine

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/xerrors"

	"github.com/coder/coder/coder-sandbox/hostvm"
	sandboxproxy "github.com/coder/coder/coder-sandbox/proxy"
)

const embeddedMicroVMCleanupTimeout = time.Minute

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

	destination := options.Destination
	if strings.TrimSpace(destination.AllowPrivateHost) == "" {
		destination.AllowPrivateHost = agentURL.Hostname()
	}
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
			Mounts: []hostvm.Mount{{
				Source: binaryPath, Target: embeddedCoderGuestPath,
				ReadOnly: true, Nosuid: true, Nodev: true,
			}},
		},
		agentCommand: embeddedAgentCommand(agentURL.String(), options.AgentToken, options.SessionToken),
		evaluator:    evaluator,
		recorder:     recorder,
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

func embeddedAgentCommand(agentURL, agentToken, sessionToken string) string {
	environment := "CODER_AGENT_URL=" + shellQuote(agentURL) +
		" CODER_AGENT_TOKEN=" + shellQuote(agentToken)
	if sessionToken != "" {
		environment += " CODER_SESSION_TOKEN=" + shellQuote(sessionToken)
	}
	command := environment + " exec " + embeddedCoderGuestPath + " agent"
	return "setsid sh -c " + shellQuote(command) +
		" </dev/null >/tmp/coder-agent.log 2>&1 &"
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}
