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

// Install resolves the control plane exemptions and installs the IPv4 rules.
// IPv6 rules are installed best-effort with ip6tables because the proxy only
// listens on IPv4 loopback; a REDIRECT there resets IPv6 TCP, which still
// prevents bypass. Install is idempotent: chains are flushed and rebuilt.
func (e *Enforcer) Install(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.probe(ctx); err != nil {
		return err
	}
	v4, v6 := splitByFamily(e.resolveExemptions(ctx))

	if err := e.installFamily(ctx, "iptables", buildRules(e.proxyPort, v4)); err != nil {
		return xerrors.Errorf("install iptables rules: %w", err)
	}
	e.installed = true
	if err := e.installFamily(ctx, "ip6tables", buildRules(e.proxyPort, v6)); err != nil {
		e.logger.Warn(ctx, "ip6tables egress rules not installed, ipv6 is unenforced", slog.Error(err))
	}
	return nil
}

// Remove deletes the chains installed by Install. It is safe to call when
// nothing was installed and it removes whatever exists, so a crashed agent
// can clean up on the next start.
func (e *Enforcer) Remove(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.prefix == nil {
		if err := e.probe(ctx); err != nil {
			if errors.Is(err, ErrEnforcementUnavailable) && !e.installed {
				return nil
			}
			return err
		}
	}
	err := e.removeFamily(ctx, "iptables")
	if v6Err := e.removeFamily(ctx, "ip6tables"); v6Err != nil {
		e.logger.Debug(ctx, "remove ip6tables egress rules", slog.Error(v6Err))
	}
	if err != nil {
		return xerrors.Errorf("remove iptables rules: %w", err)
	}
	e.installed = false
	return nil
}

// probe determines how iptables can be invoked. Root runs it directly;
// otherwise passwordless sudo is tried. The result is cached.
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
	for _, tc := range []struct {
		table string
		chain string
		rules []rule
	}{
		{table: "nat", chain: natChain, rules: rs.nat},
		{table: "filter", chain: filterChain, rules: rs.filter},
	} {
		// -N fails when the chain already exists; the following -F makes
		// the install idempotent either way.
		_, _ = e.run(ctx, bin, "-t", tc.table, "-N", tc.chain)
		if _, err := e.run(ctx, bin, "-t", tc.table, "-F", tc.chain); err != nil {
			return err
		}
		for _, r := range tc.rules {
			if _, err := e.run(ctx, bin, append([]string{"-t", tc.table}, r...)...); err != nil {
				return err
			}
		}
		// Insert the jump at the top of OUTPUT so our policy runs before
		// anything else (for example Docker's rules), once.
		if _, err := e.run(ctx, bin, "-t", tc.table, "-C", "OUTPUT", "-j", tc.chain); err != nil {
			if _, err := e.run(ctx, bin, "-t", tc.table, "-I", "OUTPUT", "1", "-j", tc.chain); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Enforcer) removeFamily(ctx context.Context, bin string) error {
	var errs []error
	for _, tc := range []struct {
		table string
		chain string
	}{
		{table: "nat", chain: natChain},
		{table: "filter", chain: filterChain},
	} {
		// Delete every jump that references the chain, then drop it.
		// Bounded so a persistent -D failure cannot loop forever.
		for range 8 {
			if _, err := e.run(ctx, bin, "-t", tc.table, "-D", "OUTPUT", "-j", tc.chain); err != nil {
				break
			}
		}
		if _, err := e.run(ctx, bin, "-t", tc.table, "-F", tc.chain); err != nil {
			if !isNoChain(err) {
				errs = append(errs, err)
			}
			continue
		}
		if _, err := e.run(ctx, bin, "-t", tc.table, "-X", tc.chain); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// isNoChain reports whether iptables complained that the chain does not
// exist, which Remove treats as already done.
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
