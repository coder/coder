//go:build linux

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
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/user"
	"runtime"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/sys/unix"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/agent/agentexec"
)

const (
	privilegeDropHelperCommand = "privdrop-helper"
	privilegeDropShellPath     = "/bin/sh"
	linuxCapabilityBits        = 64
)

var requiredPrivilegeDropCapabilities = capabilitySet(
	unix.CAP_SETGID,
	unix.CAP_SETUID,
	unix.CAP_SETPCAP,
)

// PrivilegeDropOptions configures a confined shell launched under credentials
// that differ from the supervising agent. User, Group, and DeviceGroup accept
// either names or decimal IDs. DeviceGroup is the only supplementary group
// retained, and RetainCapabilities is empty by default.
type PrivilegeDropOptions struct {
	User               string
	Group              string
	DeviceGroup        string
	RetainCapabilities []int
	Script             string

	// HelperPath defaults to the current executable. CommandPrefix can wrap the
	// helper invocation, for example with "ip netns exec <name>".
	HelperPath    string
	CommandPrefix []string
	Execer        agentexec.Execer

	Env        []string
	Dir        string
	Stdin      io.Reader
	Stdout     io.Writer
	Stderr     io.Writer
	ExtraFiles []*os.File
}

// PrivilegeDropPreflight records the resolved target credentials and the
// security properties observed before launching the helper. EntryFound fields
// are informational because valid numeric IDs do not require passwd or group
// database entries.
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

// Supported reports whether the observed host can safely launch the requested
// confined process.
func (p *PrivilegeDropPreflight) Supported() bool {
	return p.Reason() == ""
}

// Reason returns the first reason the privilege drop is unsupported, or an
// empty string when it is supported.
func (p *PrivilegeDropPreflight) Reason() string {
	if p == nil {
		return "privilege drop preflight is required"
	}
	if p.FailureReason != "" {
		return p.FailureReason
	}
	if p.TargetUID == 0 {
		return "target UID must not be root"
	}
	if p.TargetGID == 0 {
		return "target GID must not be root"
	}
	if p.TargetUID == p.SupervisorUID {
		return "target UID matches the supervisor UID"
	}
	if p.TargetGID == p.SupervisorGID {
		return "target GID matches the supervisor GID"
	}
	if !p.UIDMapped {
		return fmt.Sprintf("target UID %d is not mapped in the current user namespace", p.TargetUID)
	}
	if !p.GIDMapped {
		return fmt.Sprintf("target GID %d is not mapped in the current user namespace", p.TargetGID)
	}
	if p.DeviceGroupRequested && !p.DeviceGIDMapped {
		return fmt.Sprintf("device GID %d is not mapped in the current user namespace", p.DeviceGID)
	}
	if !p.SupplementaryGroupsSupported {
		return "supplementary groups cannot be changed in the current user namespace"
	}
	if missing := requiredPrivilegeDropCapabilities &^ p.EffectiveCapabilities; missing != 0 {
		return fmt.Sprintf("missing effective capabilities: %s", formatCapabilities(missing))
	}
	if missing := p.RequestedCapabilities &^ p.PermittedCapabilities; missing != 0 {
		return fmt.Sprintf("requested capabilities are not permitted: %s", formatCapabilities(missing))
	}
	if missing := p.RequestedCapabilities &^ p.BoundingCapabilities; missing != 0 {
		return fmt.Sprintf("requested capabilities are outside the bounding set: %s", formatCapabilities(missing))
	}
	if !p.ShellExecutable {
		return privilegeDropShellPath + " is missing or not executable"
	}
	if !p.NoNewPrivilegesSupported {
		return "PR_SET_NO_NEW_PRIVS is not supported"
	}
	return ""
}

// PreflightPrivilegeDrop resolves the requested credentials and verifies that
// the current Linux process can perform every step required by the helper.
func PreflightPrivilegeDrop(options PrivilegeDropOptions) PrivilegeDropPreflight {
	preflight := PrivilegeDropPreflight{
		TargetUID:     -1,
		TargetGID:     -1,
		DeviceGID:     -1,
		SupervisorUID: os.Geteuid(),
		SupervisorGID: os.Getegid(),
	}

	resolvedUser, err := resolvePrivilegeDropUser(options.User)
	if err != nil {
		preflight.FailureReason = err.Error()
		return preflight
	}
	preflight.TargetUID = resolvedUser.uid
	preflight.UserEntryFound = resolvedUser.entryFound

	resolvedGroup, err := resolvePrivilegeDropGroup(options.Group, resolvedUser)
	if err != nil {
		preflight.FailureReason = err.Error()
		return preflight
	}
	preflight.TargetGID = resolvedGroup.gid
	preflight.GroupEntryFound = resolvedGroup.entryFound

	if strings.TrimSpace(options.DeviceGroup) != "" {
		preflight.DeviceGroupRequested = true
		deviceGroup, resolveErr := resolveGroupIdentifier(options.DeviceGroup)
		if resolveErr != nil {
			preflight.FailureReason = xerrors.Errorf("resolve device group: %w", resolveErr).Error()
			return preflight
		}
		preflight.DeviceGID = deviceGroup.gid
		preflight.DeviceGroupEntryFound = deviceGroup.entryFound
	}

	preflight.LastCapability, err = readLastCapability()
	if err != nil {
		preflight.FailureReason = err.Error()
		return preflight
	}
	preflight.RequestedCapabilities, err = capabilitiesMask(options.RetainCapabilities, preflight.LastCapability)
	if err != nil {
		preflight.FailureReason = err.Error()
		return preflight
	}

	preflight.UIDMapped, err = identifierMapped("/proc/self/uid_map", preflight.TargetUID)
	if err != nil {
		preflight.FailureReason = err.Error()
		return preflight
	}
	preflight.GIDMapped, err = identifierMapped("/proc/self/gid_map", preflight.TargetGID)
	if err != nil {
		preflight.FailureReason = err.Error()
		return preflight
	}
	if preflight.DeviceGroupRequested {
		preflight.DeviceGIDMapped, err = identifierMapped("/proc/self/gid_map", preflight.DeviceGID)
		if err != nil {
			preflight.FailureReason = err.Error()
			return preflight
		}
	}

	preflight.SupplementaryGroupsSupported, err = supplementaryGroupsSupported()
	if err != nil {
		preflight.FailureReason = err.Error()
		return preflight
	}
	preflight.EffectiveCapabilities, preflight.PermittedCapabilities,
		preflight.BoundingCapabilities, err = readProcessCapabilities()
	if err != nil {
		preflight.FailureReason = err.Error()
		return preflight
	}

	preflight.ShellExecutable = unix.Access(privilegeDropShellPath, unix.X_OK) == nil
	_, _, errno := unix.Syscall6(unix.SYS_PRCTL, unix.PR_GET_NO_NEW_PRIVS, 0, 0, 0, 0, 0)
	preflight.NoNewPrivilegesSupported = errno == 0
	return preflight
}

// ConfinedCommand constructs an unstarted helper command from a successful
// preflight. The preflight argument makes command construction deterministic
// and lets callers log the exact observations used for the decision.
func ConfinedCommand(
	ctx context.Context,
	options PrivilegeDropOptions,
	preflight PrivilegeDropPreflight,
) (*exec.Cmd, error) {
	if !preflight.Supported() {
		return nil, xerrors.Errorf("privilege drop is unsupported: %s", preflight.Reason())
	}
	if strings.IndexByte(options.Script, 0) >= 0 {
		return nil, xerrors.New("confined script contains a NUL byte")
	}
	for index, file := range options.ExtraFiles {
		if file == nil {
			return nil, xerrors.Errorf("extra file %d is nil", index)
		}
	}

	helperPath := options.HelperPath
	if helperPath == "" {
		var err error
		helperPath, err = os.Executable()
		if err != nil {
			return nil, xerrors.Errorf("find privilege drop helper executable: %w", err)
		}
	}
	execer := options.Execer
	if execer == nil {
		execer = agentexec.DefaultExecer
	}

	config := privilegeDropConfig{
		UID:                preflight.TargetUID,
		GID:                preflight.TargetGID,
		Capabilities:       preflight.RequestedCapabilities,
		LastCapability:     preflight.LastCapability,
		AllowedDescriptors: allowedFileDescriptors(len(options.ExtraFiles)),
		Script:             options.Script,
	}
	if preflight.DeviceGroupRequested {
		config.DeviceGID = new(preflight.DeviceGID)
	}
	encodedConfig, err := json.Marshal(config)
	if err != nil {
		return nil, xerrors.Errorf("encode privilege drop helper configuration: %w", err)
	}

	helperArgs := []string{privilegeDropHelperCommand, string(encodedConfig)}
	commandPath := helperPath
	commandArgs := helperArgs
	if len(options.CommandPrefix) > 0 {
		commandPath = options.CommandPrefix[0]
		commandArgs = append(slices.Clone(options.CommandPrefix[1:]), helperPath)
		commandArgs = append(commandArgs, helperArgs...)
	}

	command := execer.CommandContext(ctx, commandPath, commandArgs...)
	command.Env = slices.Clone(options.Env)
	command.Dir = options.Dir
	command.Stdin = options.Stdin
	command.Stdout = options.Stdout
	command.Stderr = options.Stderr
	command.ExtraFiles = slices.Clone(options.ExtraFiles)
	return command, nil
}

// LaunchConfined preflights, constructs, and starts a confined shell. The
// caller owns Wait and process cleanup for the returned command.
func LaunchConfined(ctx context.Context, options PrivilegeDropOptions) (*exec.Cmd, error) {
	preflight := PreflightPrivilegeDrop(options)
	command, err := ConfinedCommand(ctx, options, preflight)
	if err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, xerrors.Errorf("start confined command: %w", err)
	}
	return command, nil
}

// RunPrivilegeDropHelper applies the security-sensitive credential and file
// descriptor changes, then replaces the helper with /bin/sh. It is intended
// only for the hidden coder helper subcommand.
func RunPrivilegeDropHelper(args []string) error {
	// Capability state is per-thread. Keep every operation and the final exec
	// on one OS thread in this dedicated helper process.
	runtime.LockOSThread()

	if len(args) != 1 {
		return xerrors.Errorf("privilege drop helper requires one configuration argument, got %d", len(args))
	}
	var config privilegeDropConfig
	if err := json.Unmarshal([]byte(args[0]), &config); err != nil {
		return xerrors.Errorf("decode privilege drop helper configuration: %w", err)
	}
	if err := validatePrivilegeDropConfig(config); err != nil {
		return err
	}
	if err := closeUnlistedFileDescriptors(config.AllowedDescriptors); err != nil {
		return err
	}

	if err := unix.Prctl(unix.PR_SET_KEEPCAPS, 1, 0, 0, 0); err != nil {
		return xerrors.Errorf("retain capabilities while changing credentials: %w", err)
	}
	supplementaryGroups := []int{}
	if config.DeviceGID != nil {
		supplementaryGroups = append(supplementaryGroups, *config.DeviceGID)
	}
	if err := unix.Setgroups(supplementaryGroups); err != nil {
		return xerrors.Errorf("set supplementary groups: %w", err)
	}
	if err := unix.Setresgid(config.GID, config.GID, config.GID); err != nil {
		return xerrors.Errorf("set real, effective, and saved GID: %w", err)
	}
	if err := unix.Setresuid(config.UID, config.UID, config.UID); err != nil {
		return xerrors.Errorf("set real, effective, and saved UID: %w", err)
	}

	setupCapabilities := config.Capabilities | capabilitySet(unix.CAP_SETPCAP)
	if err := setProcessCapabilities(setupCapabilities); err != nil {
		return xerrors.Errorf("activate capabilities needed to finish privilege drop: %w", err)
	}
	if err := unix.Prctl(unix.PR_SET_NO_NEW_PRIVS, 1, 0, 0, 0); err != nil {
		return xerrors.Errorf("set no_new_privs: %w", err)
	}
	if err := dropCapabilityBoundingSet(config.Capabilities, config.LastCapability); err != nil {
		return err
	}
	if err := setProcessCapabilities(config.Capabilities); err != nil {
		return xerrors.Errorf("set retained capabilities: %w", err)
	}
	if err := setAmbientCapabilities(config.Capabilities, config.LastCapability); err != nil {
		return err
	}
	if err := unix.Prctl(unix.PR_SET_KEEPCAPS, 0, 0, 0, 0); err != nil {
		return xerrors.Errorf("clear keep capabilities flag: %w", err)
	}

	return unix.Exec(
		privilegeDropShellPath,
		[]string{"sh", "-c", config.Script},
		os.Environ(),
	)
}

type privilegeDropConfig struct {
	UID                int    `json:"uid"`
	GID                int    `json:"gid"`
	DeviceGID          *int   `json:"device_gid,omitempty"`
	Capabilities       uint64 `json:"capabilities"`
	LastCapability     int    `json:"last_capability"`
	AllowedDescriptors []int  `json:"allowed_descriptors"`
	Script             string `json:"script"`
}

type resolvedPrivilegeDropUser struct {
	uid        int
	primaryGID int
	entryFound bool
}

type resolvedPrivilegeDropGroup struct {
	gid        int
	entryFound bool
}

func resolvePrivilegeDropUser(value string) (resolvedPrivilegeDropUser, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return resolvedPrivilegeDropUser{}, xerrors.New("target user is required")
	}
	uid, numeric, err := parseNumericIdentifier(value)
	if err != nil {
		return resolvedPrivilegeDropUser{}, xerrors.Errorf("invalid target user %q: %w", value, err)
	}
	if numeric {
		resolved := resolvedPrivilegeDropUser{uid: uid, primaryGID: -1}
		entry, lookupErr := user.LookupId(strconv.Itoa(uid))
		if lookupErr == nil {
			resolved.entryFound = true
			resolved.primaryGID, err = parseDatabaseIdentifier(entry.Gid, "primary GID")
			if err != nil {
				return resolvedPrivilegeDropUser{}, err
			}
		}
		return resolved, nil
	}

	entry, err := user.Lookup(value)
	if err != nil {
		return resolvedPrivilegeDropUser{}, xerrors.Errorf("lookup target user %q: %w", value, err)
	}
	uid, err = parseDatabaseIdentifier(entry.Uid, "UID")
	if err != nil {
		return resolvedPrivilegeDropUser{}, err
	}
	gid, err := parseDatabaseIdentifier(entry.Gid, "primary GID")
	if err != nil {
		return resolvedPrivilegeDropUser{}, err
	}
	return resolvedPrivilegeDropUser{uid: uid, primaryGID: gid, entryFound: true}, nil
}

func resolvePrivilegeDropGroup(
	value string,
	resolvedUser resolvedPrivilegeDropUser,
) (resolvedPrivilegeDropGroup, error) {
	value = strings.TrimSpace(value)
	if value != "" {
		group, err := resolveGroupIdentifier(value)
		if err != nil {
			return resolvedPrivilegeDropGroup{}, xerrors.Errorf("resolve target group: %w", err)
		}
		return group, nil
	}
	if resolvedUser.primaryGID < 0 {
		return resolvedPrivilegeDropGroup{}, xerrors.New("target group is required when a numeric user has no passwd entry")
	}
	resolved := resolvedPrivilegeDropGroup{gid: resolvedUser.primaryGID}
	if _, err := user.LookupGroupId(strconv.Itoa(resolved.gid)); err == nil {
		resolved.entryFound = true
	}
	return resolved, nil
}

func resolveGroupIdentifier(value string) (resolvedPrivilegeDropGroup, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return resolvedPrivilegeDropGroup{}, xerrors.New("group is required")
	}
	gid, numeric, err := parseNumericIdentifier(value)
	if err != nil {
		return resolvedPrivilegeDropGroup{}, xerrors.Errorf("invalid group %q: %w", value, err)
	}
	if numeric {
		resolved := resolvedPrivilegeDropGroup{gid: gid}
		if _, lookupErr := user.LookupGroupId(strconv.Itoa(gid)); lookupErr == nil {
			resolved.entryFound = true
		}
		return resolved, nil
	}

	entry, err := user.LookupGroup(value)
	if err != nil {
		return resolvedPrivilegeDropGroup{}, xerrors.Errorf("lookup group %q: %w", value, err)
	}
	gid, err = parseDatabaseIdentifier(entry.Gid, "GID")
	if err != nil {
		return resolvedPrivilegeDropGroup{}, err
	}
	return resolvedPrivilegeDropGroup{gid: gid, entryFound: true}, nil
}

func parseNumericIdentifier(value string) (int, bool, error) {
	for _, character := range value {
		if character < '0' || character > '9' {
			return 0, false, nil
		}
	}
	identifier, err := strconv.ParseUint(value, 10, 32)
	if err != nil {
		return 0, true, err
	}
	// #nosec G115 - ParseUint restricts the identifier to 32 bits, which fits
	// in an int on every supported Linux architecture.
	return int(identifier), true, nil
}

func parseDatabaseIdentifier(value, kind string) (int, error) {
	identifier, numeric, err := parseNumericIdentifier(value)
	if err != nil || !numeric {
		return 0, xerrors.Errorf("invalid %s %q", kind, value)
	}
	return identifier, nil
}

func identifierMapped(path string, identifier int) (bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return false, xerrors.Errorf("open user namespace map %q: %w", path, err)
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 3 {
			return false, xerrors.Errorf("parse user namespace map %q: invalid line %q", path, scanner.Text())
		}
		inside, parseErr := strconv.ParseUint(fields[0], 10, 32)
		if parseErr != nil {
			return false, xerrors.Errorf("parse user namespace map %q: %w", path, parseErr)
		}
		length, parseErr := strconv.ParseUint(fields[2], 10, 32)
		if parseErr != nil {
			return false, xerrors.Errorf("parse user namespace map %q: %w", path, parseErr)
		}
		// #nosec G115 - Resolved UIDs and GIDs are validated as non-negative
		// 32-bit identifiers before user namespace map checks.
		target := uint64(identifier)
		if target >= inside && target-inside < length {
			return true, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return false, xerrors.Errorf("read user namespace map %q: %w", path, err)
	}
	return false, nil
}

func supplementaryGroupsSupported() (bool, error) {
	contents, err := os.ReadFile("/proc/self/setgroups")
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	if err != nil {
		return false, xerrors.Errorf("read supplementary group policy: %w", err)
	}
	return strings.TrimSpace(string(contents)) != "deny", nil
}

func readProcessCapabilities() (effective, permitted, bounding uint64, err error) {
	file, err := os.Open("/proc/self/status")
	if err != nil {
		return 0, 0, 0, xerrors.Errorf("open process status: %w", err)
	}
	defer file.Close()

	values := map[string]*uint64{
		"CapEff:": &effective,
		"CapPrm:": &permitted,
		"CapBnd:": &bounding,
	}
	found := make(map[string]bool, len(values))
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) != 2 {
			continue
		}
		destination, ok := values[fields[0]]
		if !ok {
			continue
		}
		*destination, err = strconv.ParseUint(fields[1], 16, 64)
		if err != nil {
			return 0, 0, 0, xerrors.Errorf("parse %s from process status: %w", fields[0], err)
		}
		found[fields[0]] = true
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, 0, xerrors.Errorf("read process status: %w", err)
	}
	for name := range values {
		if !found[name] {
			return 0, 0, 0, xerrors.Errorf("process status does not contain %s", name)
		}
	}
	return effective, permitted, bounding, nil
}

func readLastCapability() (int, error) {
	contents, err := os.ReadFile("/proc/sys/kernel/cap_last_cap")
	if err != nil {
		return 0, xerrors.Errorf("read last Linux capability: %w", err)
	}
	lastCapability, err := strconv.Atoi(strings.TrimSpace(string(contents)))
	if err != nil {
		return 0, xerrors.Errorf("parse last Linux capability: %w", err)
	}
	if lastCapability < 0 || lastCapability >= linuxCapabilityBits {
		return 0, xerrors.Errorf(
			"kernel capability range 0-%d exceeds the supported 64-bit mask",
			lastCapability,
		)
	}
	return lastCapability, nil
}

func capabilitySet(capabilities ...int) uint64 {
	var mask uint64
	for _, capability := range capabilities {
		// #nosec G115 - Capability constants are non-negative and below the
		// 64-bit Linux capability mask limit.
		mask |= uint64(1) << uint(capability)
	}
	return mask
}

func capabilitiesMask(capabilities []int, lastCapability int) (uint64, error) {
	var mask uint64
	for _, capability := range capabilities {
		if capability < 0 || capability > lastCapability {
			return 0, xerrors.Errorf(
				"requested capability %d is outside the kernel range 0-%d",
				capability,
				lastCapability,
			)
		}
		mask |= capabilitySet(capability)
	}
	return mask, nil
}

func formatCapabilities(mask uint64) string {
	capabilities := make([]string, 0, linuxCapabilityBits)
	for capability := range linuxCapabilityBits {
		if mask&capabilitySet(capability) != 0 {
			capabilities = append(capabilities, strconv.Itoa(capability))
		}
	}
	return strings.Join(capabilities, ",")
}

func allowedFileDescriptors(extraFiles int) []int {
	descriptors := make([]int, 0, 3+extraFiles)
	for descriptor := range 3 + extraFiles {
		descriptors = append(descriptors, descriptor)
	}
	return descriptors
}

func validatePrivilegeDropConfig(config privilegeDropConfig) error {
	if config.UID <= 0 {
		return xerrors.New("target UID must be a non-root 32-bit identifier")
	}
	if config.GID <= 0 {
		return xerrors.New("target GID must be a non-root 32-bit identifier")
	}
	if config.UID > math.MaxUint32 || config.GID > math.MaxUint32 {
		return xerrors.New("target UID and GID must fit in 32 bits")
	}
	if config.UID == os.Geteuid() {
		return xerrors.New("target UID matches the helper UID")
	}
	if config.GID == os.Getegid() {
		return xerrors.New("target GID matches the helper GID")
	}
	if config.DeviceGID != nil {
		if *config.DeviceGID <= 0 || *config.DeviceGID > math.MaxUint32 {
			return xerrors.New("device GID must be a non-root 32-bit identifier")
		}
	}
	if config.LastCapability < 0 || config.LastCapability >= linuxCapabilityBits {
		return xerrors.Errorf("invalid last Linux capability %d", config.LastCapability)
	}
	// #nosec G115 - LastCapability is validated in the range 0-63 before
	// conversion to the unsigned shift count.
	capabilityLimit := uint(config.LastCapability + 1)
	if config.Capabilities>>capabilityLimit != 0 {
		return xerrors.New("retained capability mask exceeds the kernel capability range")
	}
	if strings.IndexByte(config.Script, 0) >= 0 {
		return xerrors.New("confined script contains a NUL byte")
	}

	descriptors := slices.Clone(config.AllowedDescriptors)
	slices.Sort(descriptors)
	descriptors = slices.Compact(descriptors)
	if len(descriptors) != len(config.AllowedDescriptors) {
		return xerrors.New("file descriptor allowlist contains duplicates")
	}
	if len(descriptors) < 3 || descriptors[0] != 0 || descriptors[1] != 1 || descriptors[2] != 2 {
		return xerrors.New("file descriptor allowlist must include standard input, output, and error")
	}
	for _, descriptor := range descriptors {
		if descriptor < 0 {
			return xerrors.Errorf("invalid allowed file descriptor %d", descriptor)
		}
	}
	return nil
}

func closeUnlistedFileDescriptors(allowedDescriptors []int) error {
	allowed := make(map[int]struct{}, len(allowedDescriptors))
	for _, descriptor := range allowedDescriptors {
		allowed[descriptor] = struct{}{}
	}
	entries, err := os.ReadDir("/proc/self/fd")
	if err != nil {
		return xerrors.Errorf("enumerate open file descriptors: %w", err)
	}
	for _, entry := range entries {
		descriptor, parseErr := strconv.Atoi(entry.Name())
		if parseErr != nil {
			continue
		}
		if _, ok := allowed[descriptor]; ok {
			if err := clearCloseOnExec(descriptor); err != nil {
				return err
			}
			continue
		}
		if err := unix.Close(descriptor); err != nil && !errors.Is(err, unix.EBADF) {
			return xerrors.Errorf("close file descriptor %d: %w", descriptor, err)
		}
	}
	return nil
}

func clearCloseOnExec(descriptor int) error {
	// #nosec G115 - File descriptors are non-negative ints returned by procfs.
	fd := uintptr(descriptor)
	flags, err := unix.FcntlInt(fd, unix.F_GETFD, 0)
	if err != nil {
		return xerrors.Errorf("read flags for allowed file descriptor %d: %w", descriptor, err)
	}
	if _, err := unix.FcntlInt(fd, unix.F_SETFD, flags&^unix.FD_CLOEXEC); err != nil {
		return xerrors.Errorf("clear close-on-exec for allowed file descriptor %d: %w", descriptor, err)
	}
	return nil
}

func setProcessCapabilities(mask uint64) error {
	header := unix.CapUserHeader{Version: unix.LINUX_CAPABILITY_VERSION_3}
	// #nosec G115 - These conversions select the low and high 32-bit words
	// from the validated 64-bit Linux capability mask.
	low := uint32(mask)
	// #nosec G115 - The shift selects only the high 32-bit word.
	high := uint32(mask >> 32)
	data := [2]unix.CapUserData{
		{
			Effective:   low,
			Permitted:   low,
			Inheritable: low,
		},
		{
			Effective:   high,
			Permitted:   high,
			Inheritable: high,
		},
	}
	return unix.Capset(&header, &data[0])
}

func dropCapabilityBoundingSet(retained uint64, lastCapability int) error {
	for capability := range lastCapability + 1 {
		if retained&capabilitySet(capability) != 0 {
			continue
		}
		// #nosec G115 - The capability index is validated against the kernel's
		// cap_last_cap value and is therefore a non-negative uintptr value.
		argument := uintptr(capability)
		if err := unix.Prctl(unix.PR_CAPBSET_DROP, argument, 0, 0, 0); err != nil {
			return xerrors.Errorf("drop capability %d from bounding set: %w", capability, err)
		}
	}
	return nil
}

func setAmbientCapabilities(retained uint64, lastCapability int) error {
	if err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_CLEAR_ALL, 0, 0, 0); err != nil {
		return xerrors.Errorf("clear ambient capabilities: %w", err)
	}
	for capability := range lastCapability + 1 {
		if retained&capabilitySet(capability) == 0 {
			continue
		}
		// #nosec G115 - The capability index is validated against the kernel's
		// cap_last_cap value and is therefore a non-negative uintptr value.
		argument := uintptr(capability)
		if err := unix.Prctl(unix.PR_CAP_AMBIENT, unix.PR_CAP_AMBIENT_RAISE, argument, 0, 0); err != nil {
			return xerrors.Errorf("retain ambient capability %d: %w", capability, err)
		}
	}
	return nil
}
