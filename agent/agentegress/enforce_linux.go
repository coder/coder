//go:build linux

package agentegress

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// Install resolves exemptions and installs enforcement for every enabled
// address family. Transparent UDP is attempted first and falls back to
// REDIRECT when TPROXY or policy routing is unavailable.
func (e *Enforcer) Install(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.installed {
		return nil
	}

	if err := e.probe(ctx); err != nil {
		return err
	}
	exemptions := e.resolveExemptions(ctx)
	resolvers := e.resolvers
	if resolvers == nil {
		resolvers = systemResolvers()
	}

	var transparentErr error
	if !udpTransparentReady(e.ports.udp) {
		transparentErr = xerrors.New("UDP capture socket lacks IP_TRANSPARENT")
	} else {
		transparentErr = e.installTPROXY(ctx, exemptions)
	}
	e.transparent = transparentErr == nil
	if transparentErr != nil {
		_ = e.removeTPROXY(ctx)
		e.logger.Warn(ctx, "transparent udp unavailable, non-dns udp is dropped because REDIRECT loses the original destination",
			slog.Error(transparentErr))
	}
	mode := udpModeTProxy
	if !e.transparent {
		mode = udpModeRedirect
	}
	if err := e.installFamily(ctx, "iptables", buildRules(e.ports, exemptions, resolvers, ipv4, mode)); err != nil {
		_ = e.removeTPROXY(ctx)
		return xerrors.Errorf("install iptables rules: %w", err)
	}
	e.installed = true
	if err := e.installFamily(ctx, "ip6tables", buildRules(e.ports, exemptions, resolvers, ipv6, udpModeRedirect)); err != nil {
		if cleanupErr := e.handleIPv6InstallFailure(ctx, err); cleanupErr != nil {
			return cleanupErr
		}
	}
	if e.lockdown {
		if err := e.lockdownCapabilities(); err != nil {
			return xerrors.Errorf("lock down CAP_NET_ADMIN for child processes: %w", err)
		}
		// Children cannot regain CAP_NET_ADMIN after this point. Cleanup by
		// exec is also impossible, so rules remain until the container's
		// network namespace exits.
		e.lockedDown = true
	}
	return nil
}

// Remove deletes rules installed by Install. After capability lockdown it
// skips cleanup because subprocesses cannot regain CAP_NET_ADMIN.
func (e *Enforcer) Remove(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.lockedDown {
		e.logger.Info(ctx, "egress enforcement rules persist until the container exits")
		return nil
	}
	if err := e.probe(ctx); err != nil {
		if errors.Is(err, ErrEnforcementUnavailable) && !e.installed {
			return nil
		}
		return err
	}
	var errs []error
	if err := e.removeFamily(ctx, "iptables"); err != nil {
		errs = append(errs, xerrors.Errorf("remove iptables rules: %w", err))
	}
	if err := e.removeTPROXY(ctx); err != nil {
		errs = append(errs, xerrors.Errorf("remove transparent udp rules: %w", err))
	}
	if err := e.removeFamily(ctx, "ip6tables"); err != nil {
		e.logger.Debug(ctx, "remove ip6tables egress rules", slog.Error(err))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	e.installed = false
	e.transparent = false
	return nil
}

// probe determines how privileged networking commands can be invoked.
func (e *Enforcer) probe(ctx context.Context) error {
	if e.prefix != nil {
		return nil
	}
	candidates := [][]string{{"sudo", "-n"}}
	if os.Geteuid() == 0 {
		candidates = [][]string{{}}
	}
	var errs []error
	for _, prefix := range candidates {
		_, err := e.runWith(ctx, prefix, "iptables", "-t", "nat", "-L", "OUTPUT", "-n")
		if err == nil {
			e.prefix = prefix
			return nil
		}
		errs = append(errs, err)
	}
	return xerrors.Errorf("%w: %w", ErrEnforcementUnavailable, errors.Join(errs...))
}

func (e *Enforcer) handleIPv6InstallFailure(ctx context.Context, installErr error) error {
	enabled, err := e.ipv6Enabled()
	if err != nil {
		e.logger.Warn(ctx, "cannot determine whether ipv6 needs enforcement", slog.Error(err))
		enabled = true
	}
	if !enabled {
		e.logger.Debug(ctx, "ip6tables unavailable but ipv6 is disabled", slog.Error(installErr))
		return nil
	}
	disableErr := e.disableIPv6()
	if disableErr == nil {
		e.logger.Warn(ctx, "ip6tables unavailable, disabled ipv6 to keep egress enforcement fail closed",
			slog.Error(installErr))
		return nil
	}
	e.logger.Warn(ctx, "ip6tables unavailable and ipv6 could not be disabled, removing ipv4 enforcement",
		slog.Error(xerrors.Errorf("install ip6tables: %w; disable ipv6: %w", installErr, disableErr)))
	cleanupErr := errors.Join(e.removeFamily(ctx, "iptables"), e.removeTPROXY(ctx), e.removeFamily(ctx, "ip6tables"))
	e.installed = false
	e.transparent = false
	return xerrors.Errorf("ipv6 enforcement unavailable: install ip6tables: %w; disable ipv6: %w; clean up ipv4 rules: %w", installErr, disableErr, cleanupErr)
}

func (e *Enforcer) ipv6Enabled() (bool, error) {
	disabled, err := e.readFile("/proc/sys/net/ipv6/conf/all/disable_ipv6")
	if err != nil {
		return false, xerrors.Errorf("read ipv6 disable sysctl: %w", err)
	}
	if strings.TrimSpace(string(disabled)) != "0" {
		return false, nil
	}
	interfaces, err := e.readFile("/proc/net/if_inet6")
	if err != nil {
		return false, xerrors.Errorf("read ipv6 interfaces: %w", err)
	}
	return hasNonLoopbackIPv6(interfaces), nil
}

func hasNonLoopbackIPv6(contents []byte) bool {
	for line := range strings.Lines(string(contents)) {
		fields := strings.Fields(line)
		if len(fields) >= 6 && fields[5] != "lo" {
			return true
		}
	}
	return false
}

func (e *Enforcer) disableIPv6() error {
	var errs []error
	for _, path := range []string{
		"/proc/sys/net/ipv6/conf/all/disable_ipv6",
		"/proc/sys/net/ipv6/conf/default/disable_ipv6",
	} {
		if err := e.writeFile(path, []byte("1\n"), 0o644); err != nil {
			errs = append(errs, xerrors.Errorf("write %s: %w", path, err))
		}
	}
	return errors.Join(errs...)
}
func (e *Enforcer) installFamily(ctx context.Context, bin string, rs ruleSet) error {
	for _, tc := range managedChains {
		_, _ = e.run(ctx, bin, "-t", tc.table, "-N", tc.chain)
		if _, err := e.run(ctx, bin, "-t", tc.table, "-F", tc.chain); err != nil {
			return err
		}
		for _, r := range rs[tc.table] {
			if _, err := e.run(ctx, bin, append([]string{"-t", tc.table}, r...)...); err != nil {
				return err
			}
		}
		if _, err := e.run(ctx, bin, "-t", tc.table, "-C", tc.parent, "-j", tc.chain); err != nil {
			if _, err := e.run(ctx, bin, "-t", tc.table, "-I", tc.parent, "1", "-j", tc.chain); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Enforcer) installTPROXY(ctx context.Context, exemptions []resolvedExemption) error {
	rs := buildTPROXYRules(e.ports, exemptions)
	for _, chain := range []string{tproxyOutputChain, tproxyPreroutingChain} {
		_, _ = e.run(ctx, "iptables", "-t", "mangle", "-N", chain)
		if _, err := e.run(ctx, "iptables", "-t", "mangle", "-F", chain); err != nil {
			return xerrors.Errorf("flush mangle chain %s: %w", chain, err)
		}
	}
	for _, r := range rs["output"] {
		if _, err := e.run(ctx, "iptables", append([]string{"-t", "mangle"}, r...)...); err != nil {
			return xerrors.Errorf("install UDP mark rule: %w", err)
		}
	}
	for _, r := range rs["prerouting"] {
		if _, err := e.run(ctx, "iptables", append([]string{"-t", "mangle"}, r...)...); err != nil {
			return xerrors.Errorf("install TPROXY rule: %w", err)
		}
	}
	for _, jump := range []struct{ parent, chain string }{{"OUTPUT", tproxyOutputChain}, {"PREROUTING", tproxyPreroutingChain}} {
		if _, err := e.run(ctx, "iptables", "-t", "mangle", "-C", jump.parent, "-j", jump.chain); err != nil {
			if _, err := e.run(ctx, "iptables", "-t", "mangle", "-I", jump.parent, "1", "-j", jump.chain); err != nil {
				return xerrors.Errorf("install mangle %s jump: %w", jump.parent, err)
			}
		}
	}
	if _, err := e.run(ctx, "ip", "-4", "route", "replace", "local", "0.0.0.0/0", "dev", "lo", "table", tproxyTable); err != nil {
		return xerrors.Errorf("install TPROXY route table %s: %w", tproxyTable, err)
	}
	return e.ensureIPRule(ctx)
}

func (e *Enforcer) ensureIPRule(ctx context.Context) error {
	out, err := e.run(ctx, "ip", "-4", "rule", "show")
	if err != nil {
		return xerrors.Errorf("list IPv4 policy rules: %w", err)
	}
	needle := "fwmark " + tproxyMark + " lookup " + tproxyTable
	if strings.Contains(out, needle) || strings.Contains(out, "fwmark 0x434f lookup "+tproxyTable) {
		return nil
	}
	if _, err := e.run(ctx, "ip", "-4", "rule", "add", "fwmark", tproxyMark, "lookup", tproxyTable); err != nil {
		return xerrors.Errorf("install TPROXY policy rule: %w", err)
	}
	return nil
}

func (e *Enforcer) removeFamily(ctx context.Context, bin string) error {
	var errs []error
	for _, tc := range managedChains {
		if err := e.removeChain(ctx, bin, tc.table, tc.parent, tc.chain); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (e *Enforcer) removeTPROXY(ctx context.Context) error {
	var errs []error
	for _, tc := range []struct{ parent, chain string }{{"OUTPUT", tproxyOutputChain}, {"PREROUTING", tproxyPreroutingChain}} {
		if err := e.removeChain(ctx, "iptables", "mangle", tc.parent, tc.chain); err != nil {
			errs = append(errs, err)
		}
	}
	for range 8 {
		if _, err := e.run(ctx, "ip", "-4", "rule", "del", "fwmark", tproxyMark, "lookup", tproxyTable); err != nil {
			break
		}
	}
	if _, err := e.run(ctx, "ip", "-4", "route", "flush", "table", tproxyTable); err != nil && !strings.Contains(err.Error(), "FIB table does not exist") {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

func (e *Enforcer) removeChain(ctx context.Context, bin, table, parent, chain string) error {
	for range 8 {
		if _, err := e.run(ctx, bin, "-t", table, "-D", parent, "-j", chain); err != nil {
			break
		}
	}
	if _, err := e.run(ctx, bin, "-t", table, "-F", chain); err != nil {
		if isNoChain(err) {
			return nil
		}
		return err
	}
	_, err := e.run(ctx, bin, "-t", table, "-X", chain)
	return err
}

func isNoChain(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "No chain/target/match by that name") ||
		strings.Contains(msg, "does not exist")
}

func (e *Enforcer) run(ctx context.Context, bin string, args ...string) (string, error) {
	return e.runWith(ctx, e.prefix, bin, args...)
}

func (e *Enforcer) runWith(ctx context.Context, prefix []string, bin string, args ...string) (string, error) {
	argv := append(slices.Clone(prefix), bin)
	argv = append(argv, args...)
	cmd := e.execer.CommandContext(ctx, argv[0], argv[1:]...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return string(out), xerrors.Errorf("%s: %w: %s", strings.Join(argv, " "), err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
