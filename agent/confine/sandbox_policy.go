package confine

import (
	"cmp"
	"context"
	"net"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"gopkg.in/yaml.v3"

	"cdr.dev/slog/v3"
	"github.com/coder/coder/coder-sandbox/policy"
	"github.com/coder/coder/v2/codersdk"
)

const (
	sandboxPolicyResolveTimeout      = 5 * time.Second
	sandboxPolicyReloadScriptTimeout = 30 * time.Second
)

type sandboxRuntimeNetwork struct {
	Reload  string        `yaml:"reload"`
	Default policy.Action `yaml:"default"`
	Mode    policy.Mode   `yaml:"mode"`
	Rules   []policy.Rule `yaml:"rules"`
}

type sandboxPolicyResolver interface {
	LookupNetIP(context.Context, string, string) ([]netip.Addr, error)
}

func (c *SandboxController) exportSandboxPolicy(
	ctx context.Context,
	sandboxID uuid.UUID,
	egressPolicy codersdk.AIEgressPolicy,
) {
	policyPath := strings.TrimSpace(c.options.Declaration.PolicyFile)
	if policyPath == "" {
		return
	}

	network, resolveErr := buildSandboxRuntimeNetwork(
		ctx,
		egressPolicy,
		c.options.AccessURL,
		net.DefaultResolver,
	)
	if resolveErr != nil {
		c.options.Logger.Warn(ctx, "resolve AI sandbox access URL for policy export",
			slog.F("host", c.options.AccessURL.Hostname()),
			slog.Error(resolveErr),
		)
	}
	data, err := yaml.Marshal(network)
	if err != nil {
		c.signalDegraded("AI sandbox policy translation failed (degraded)", err)
		return
	}
	if err := writeSandboxPolicyFile(policyPath, data); err != nil {
		c.signalDegraded("AI sandbox policy file write failed (degraded)", err)
		return
	}
	c.options.Logger.Info(ctx, "exported AI sandbox policy",
		slog.F("path", policyPath),
		slog.F("revision", egressPolicy.Revision),
		slog.F("rule_count", len(network.Rules)),
	)

	reloadScript := strings.TrimSpace(c.options.Declaration.PolicyReloadScript)
	if reloadScript == "" {
		return
	}
	env := append([]string(nil), os.Environ()...)
	env = setEnv(env, EnvSandboxID, sandboxID.String())
	env = setEnv(env, EnvAISandboxPolicyFile, policyPath)
	if err := c.runScript(ctx, "policy reload", reloadScript, env, sandboxPolicyReloadScriptTimeout); err != nil {
		c.signalDegraded("AI sandbox policy reload script failed; updates remain enabled (degraded)", err)
	}
}

func buildSandboxRuntimeNetwork(
	ctx context.Context,
	egressPolicy codersdk.AIEgressPolicy,
	accessURL *url.URL,
	resolver sandboxPolicyResolver,
) (sandboxRuntimeNetwork, error) {
	port, err := accessURLPort(accessURL)
	if err != nil {
		return sandboxRuntimeNetwork{}, err
	}

	rules := make([]policy.Rule, 0, len(egressPolicy.Rules)+2)
	for _, rule := range egressPolicy.Rules {
		ports := sandboxRulePorts(rule.Ports)
		if len(rule.Ports) > 0 && len(ports) == 0 {
			continue
		}
		// coder/sandbox treats a leading "*." as an arbitrary-depth suffix
		// match. The Coder egress policy matches exactly one leading label, so
		// passing the pattern through widens the match in microVM mode.
		rules = append(rules, sandboxAllowRule(rule.Host, ports))
	}

	accessHost := accessURL.Hostname()
	// #nosec G115 - accessURLPort validates that the port fits in uint16.
	accessPort := uint16(port)
	accessPorts := []uint16{accessPort}
	accessRule := sandboxAllowRule(accessHost, accessPorts)
	rules = append(rules, accessRule)
	if accessRule.CIDR != "" {
		return newSandboxRuntimeNetwork(rules), nil
	}

	resolveCtx, cancel := context.WithTimeout(ctx, sandboxPolicyResolveTimeout)
	addresses, resolveErr := resolver.LookupNetIP(resolveCtx, "ip", accessHost)
	cancel()
	for _, address := range addresses {
		address = address.Unmap()
		if !address.IsPrivate() && !address.IsLoopback() {
			continue
		}
		rules = append(rules, policy.Rule{
			CIDR:   netip.PrefixFrom(address, address.BitLen()).String(),
			Ports:  append([]uint16(nil), accessPorts...),
			Action: policy.ActionAllow,
			TLS:    policy.TLSPassthrough,
		})
	}
	return newSandboxRuntimeNetwork(rules), resolveErr
}

func newSandboxRuntimeNetwork(rules []policy.Rule) sandboxRuntimeNetwork {
	slices.SortFunc(rules, compareSandboxRules)
	rules = slices.CompactFunc(rules, func(a, b policy.Rule) bool {
		return compareSandboxRules(a, b) == 0
	})
	return sandboxRuntimeNetwork{
		Reload:  "watch",
		Default: policy.ActionDeny,
		Mode:    policy.ModeEnforce,
		Rules:   rules,
	}
}

func sandboxAllowRule(host string, ports []uint16) policy.Rule {
	rule := policy.Rule{
		Ports:  append([]uint16(nil), ports...),
		Action: policy.ActionAllow,
		TLS:    policy.TLSPassthrough,
	}
	if address, err := netip.ParseAddr(host); err == nil {
		address = address.Unmap()
		rule.CIDR = netip.PrefixFrom(address, address.BitLen()).String()
	} else {
		rule.Host = host
	}
	return rule
}

func sandboxRulePorts(ports []int) []uint16 {
	if len(ports) == 0 {
		return []uint16{defaultHTTPPort, defaultHTTPSPort}
	}
	translated := make([]uint16, 0, len(ports))
	for _, port := range ports {
		if validPort(port) {
			// #nosec G115 - validPort restricts the value to the uint16 range.
			translated = append(translated, uint16(port))
		}
	}
	slices.Sort(translated)
	return slices.Compact(translated)
}

func compareSandboxRules(a, b policy.Rule) int {
	return cmp.Or(
		cmp.Compare(a.Host, b.Host),
		cmp.Compare(a.CIDR, b.CIDR),
		slices.Compare(a.Ports, b.Ports),
		cmp.Compare(a.Action, b.Action),
		cmp.Compare(a.Name, b.Name),
		cmp.Compare(a.TLS, b.TLS),
	)
}

func writeSandboxPolicyFile(path string, data []byte) (retErr error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return xerrors.Errorf("create policy directory: %w", err)
	}
	temp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return xerrors.Errorf("create temporary policy file: %w", err)
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		if retErr != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return xerrors.Errorf("set temporary policy file permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		return xerrors.Errorf("write temporary policy file: %w", err)
	}
	if err := temp.Close(); err != nil {
		return xerrors.Errorf("close temporary policy file: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return xerrors.Errorf("replace policy file: %w", err)
	}
	return nil
}
