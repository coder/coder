//go:build !linux

// Package confine contains the host-side enforcement used for confined
// sandbox scripts. Firewall rules are enforced by the kernel, so a confined
// process cannot ignore them. Privilege dropping addresses a separate trust
// boundary: the supervisor remains outside the network namespace with host
// namespace sockets and agent credentials. A process with the supervisor's
// UID or GID could recover those resources through process inspection, file
// descriptor access, or user-owned files and sockets. The launcher therefore
// runs confined scripts under distinct credentials. Network namespace entry
// does not change credentials, so this separation must be explicit.
package confine

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"

	"github.com/coder/coder/v2/agent/agentexec"
)

const privilegeDropUnsupportedReason = "privilege dropping requires Linux"

// PrivilegeDropOptions configures a confined shell launched under credentials
// that differ from the supervising agent. Privilege dropping is unsupported on
// non-Linux systems.
type PrivilegeDropOptions struct {
	User               string
	Group              string
	DeviceGroup        string
	RetainCapabilities []int
	Script             string
	HelperPath         string
	CommandPrefix      []string
	Execer             agentexec.Execer
	Env                []string
	Dir                string
	Stdin              io.Reader
	Stdout             io.Writer
	Stderr             io.Writer
	ExtraFiles         []*os.File
}

// PrivilegeDropPreflight records why privilege dropping is unavailable on the
// current non-Linux system.
type PrivilegeDropPreflight struct {
	TargetUID int
	TargetGID int
	DeviceGID int

	UserEntryFound        bool
	GroupEntryFound       bool
	DeviceGroupRequested  bool
	DeviceGroupEntryFound bool

	SupervisorUID int
	SupervisorGID int

	UIDMapped       bool
	GIDMapped       bool
	DeviceGIDMapped bool

	SupplementaryGroupsSupported bool
	EffectiveCapabilities        uint64
	PermittedCapabilities        uint64
	BoundingCapabilities         uint64
	RequestedCapabilities        uint64
	LastCapability               int
	ShellExecutable              bool
	NoNewPrivilegesSupported     bool

	FailureReason string
}

// Supported reports false because privilege dropping requires Linux.
func (p *PrivilegeDropPreflight) Supported() bool {
	return false
}

// Reason reports that privilege dropping requires Linux.
func (p *PrivilegeDropPreflight) Reason() string {
	if p != nil && p.FailureReason != "" {
		return p.FailureReason
	}
	return privilegeDropUnsupportedReason
}

// PreflightPrivilegeDrop reports that privilege dropping requires Linux.
func PreflightPrivilegeDrop(PrivilegeDropOptions) PrivilegeDropPreflight {
	return PrivilegeDropPreflight{FailureReason: privilegeDropUnsupportedReason}
}

// ConfinedCommand reports that privilege dropping requires Linux.
func ConfinedCommand(
	context.Context,
	PrivilegeDropOptions,
	PrivilegeDropPreflight,
) (*exec.Cmd, error) {
	return nil, errors.ErrUnsupported
}

// LaunchConfined reports that privilege dropping requires Linux.
func LaunchConfined(context.Context, PrivilegeDropOptions) (*exec.Cmd, error) {
	return nil, errors.ErrUnsupported
}

// RunPrivilegeDropHelper reports that privilege dropping requires Linux.
func RunPrivilegeDropHelper([]string) error {
	return errors.ErrUnsupported
}
