//go:build linux

package nsjail

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/unix"
)

type Jailer interface {
	ConfigureHost() error
	Command(command []string) *exec.Cmd
	ConfigureHostNsCommunication(processPID int) error
	Close() error
}

type Config struct {
	Logger                           *slog.Logger
	HttpProxyPort                    int
	HomeDir                          string
	ConfigDir                        string
	CACertPath                       string
	ConfigureDNSForLocalStubResolver bool
}

// LinuxJail implements Jailer using Linux network namespaces
type LinuxJail struct {
	logger                           *slog.Logger
	vethHostName                     string // Host-side veth interface name for iptables rules
	vethJailName                     string // Jail-side veth interface name for iptables rules
	httpProxyPort                    int
	configDir                        string
	caCertPath                       string
	configureDNSForLocalStubResolver bool
}

func NewLinuxJail(config Config) (*LinuxJail, error) {
	return &LinuxJail{
		logger:                           config.Logger,
		httpProxyPort:                    config.HttpProxyPort,
		configDir:                        config.ConfigDir,
		caCertPath:                       config.CACertPath,
		configureDNSForLocalStubResolver: config.ConfigureDNSForLocalStubResolver,
	}, nil
}

// ConfigureBeforeCommandExecution prepares the jail environment before the target
// process is launched. It sets environment variables, creates the veth pair, and
// installs iptables rules on the host. At this stage, the target PID and its netns
// are not yet known.
func (l *LinuxJail) ConfigureHost() error {
	if err := l.configureHostNetworkBeforeCmdExec(); err != nil {
		return err
	}
	if err := l.configureIptables(); err != nil {
		return fmt.Errorf("failed to configure iptables: %v", err)
	}

	return nil
}

// Command returns an exec.Cmd configured to run within the network namespace.
func (l *LinuxJail) Command(command []string) *exec.Cmd {
	l.logger.Debug("Creating command with namespace")

	cmd := exec.Command(command[0], command[1:]...)
	// Set env vars for the child process; they will be inherited by the target process.
	cmd.Env = getEnvsForTargetProcess(l.configDir, l.caCertPath)
	cmd.Env = append(cmd.Env, "CHILD=true")
	cmd.Env = append(cmd.Env, fmt.Sprintf("VETH_JAIL_NAME=%v", l.vethJailName))
	if l.configureDNSForLocalStubResolver {
		cmd.Env = append(cmd.Env, "CONFIGURE_DNS_FOR_LOCAL_STUB_RESOLVER=true")
	}
	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout
	cmd.Stdin = os.Stdin

	l.logger.Debug("os.Getuid()", "os.Getuid()", os.Getuid())
	l.logger.Debug("os.Getgid()", "os.Getgid()", os.Getgid())
	currentUid := os.Getuid()
	currentGid := os.Getgid()

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: syscall.CLONE_NEWUSER | syscall.CLONE_NEWNET,
		UidMappings: []syscall.SysProcIDMap{
			{ContainerID: currentUid, HostID: currentUid, Size: 1},
		},
		GidMappings: []syscall.SysProcIDMap{
			{ContainerID: currentGid, HostID: currentGid, Size: 1},
		},
		AmbientCaps: []uintptr{unix.CAP_NET_ADMIN},
		Pdeathsig:   syscall.SIGTERM,
	}

	return cmd
}

// ConfigureHostNsCommunication finalizes host-side networking after the target
// process has started. It moves the jail-side veth into the target process's network
// namespace using the provided PID. This requires the process to be running so
// its PID (and thus its netns) are available.
func (l *LinuxJail) ConfigureHostNsCommunication(pidInt int) error {
	PID := fmt.Sprintf("%v", pidInt)

	runner := newCommandRunner([]*command{
		// Move the jail-side veth interface into the target network namespace.
		// This isolates the interface so that it becomes visible only inside the
		// jail's netns. From this point on, the jail will configure its end of
		// the veth pair (IP address, routes, etc.) independently of the host.
		newCommand(
			"Move jail-side veth into network namespace",
			exec.Command("ip", "link", "set", l.vethJailName, "netns", PID),
			[]uintptr{uintptr(unix.CAP_NET_ADMIN)},
		),
	})
	if err := runner.run(); err != nil {
		return err
	}

	return nil
}

// Close removes the network namespace and iptables rules
func (l *LinuxJail) Close() error {
	// Clean up iptables rules
	err := l.cleanupIptables()
	if err != nil {
		l.logger.Error("Failed to clean up iptables rules", "error", err)
		// Continue with other cleanup even if this fails
	}

	// Clean up networking
	err = l.cleanupNetworking()
	if err != nil {
		l.logger.Error("Failed to clean up networking", "error", err)
		// Continue with other cleanup even if this fails
	}

	return nil
}
