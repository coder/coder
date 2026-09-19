//go:build linux

package agentegress

import (
	"context"
	"errors"
	"net/netip"
	"os"
	"slices"
	"strings"

	"golang.org/x/xerrors"

	"cdr.dev/slog/v3"
)

// Install resolves exemptions and installs IPv4 enforcement. Transparent UDP
// is attempted first and falls back to REDIRECT when TPROXY or policy routing
// is unavailable. IPv6 remains best-effort REDIRECT enforcement.
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
		e.logger.Warn(ctx, "ip6tables egress rules not installed, ipv6 is unenforced", slog.Error(err))
	}
	if e.lockdown {
		if err := dropNetAdmin(e.prctl); err != nil {
			return xerrors.Errorf("drop CAP_NET_ADMIN from capability bounding set: %w", err)
		}
		// Children cannot regain CAP_NET_ADMIN after this point, even when
		// changing uid. Cleanup by exec is also impossible, so rules remain
		// until the container's network namespace exits.
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

func (e *Enforcer) installTPROXY(ctx context.Context, exemptions []netip.AddrPort) error {
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
