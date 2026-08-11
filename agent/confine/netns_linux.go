//go:build linux

package confine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/xerrors"
)

const (
	// NetNSSubnet is the fixed point-to-point subnet used between the
	// supervisor and confined child namespace. The child has no default route.
	NetNSSubnet = "100.115.92.0/30"
	netNSHostIP = "100.115.92.1"
	netNSPeerIP = "100.115.92.2"
)

type networkNamespace struct {
	name     string
	hostVeth string
	peerVeth string
	hostIP   string
	ipPath   string
}

func newNetworkNamespace(ctx context.Context) (_ *networkNamespace, retErr error) {
	ipPath, err := exec.LookPath("ip")
	if err != nil {
		return nil, xerrors.Errorf("find ip binary: %w", err)
	}
	suffix, err := randomHex(4)
	if err != nil {
		return nil, err
	}
	netns := &networkNamespace{
		name:     "coder-confine-" + suffix,
		hostVeth: "cc-h-" + suffix,
		peerVeth: "cc-n-" + suffix,
		hostIP:   netNSHostIP,
		ipPath:   ipPath,
	}
	defer func() {
		if retErr != nil {
			_ = netns.Close()
		}
	}()

	commands := [][]string{
		{"netns", "add", netns.name},
		{"link", "add", netns.hostVeth, "type", "veth", "peer", "name", netns.peerVeth},
		{"addr", "add", netNSHostIP + "/30", "dev", netns.hostVeth},
		{"link", "set", netns.hostVeth, "up"},
		{"link", "set", netns.peerVeth, "netns", netns.name},
		{"-n", netns.name, "addr", "add", netNSPeerIP + "/30", "dev", netns.peerVeth},
		{"-n", netns.name, "link", "set", netns.peerVeth, "up"},
		{"-n", netns.name, "link", "set", "lo", "up"},
	}
	for _, args := range commands {
		if err := runIP(ctx, ipPath, args...); err != nil {
			return nil, err
		}
	}

	resolverDir := filepath.Join("/etc/netns", netns.name)
	if err := os.MkdirAll(resolverDir, 0o755); err != nil {
		return nil, xerrors.Errorf("create netns resolver directory: %w", err)
	}
	// No DNS relay runs in this PoC. Direct DNS reaches the host-side veth and
	// times out, while CONNECT and HTTP proxy destinations resolve outside.
	resolver := []byte("nameserver " + netNSHostIP + "\n")
	//nolint:gosec // Resolver configuration is conventionally world-readable and contains no secrets.
	if err := os.WriteFile(filepath.Join(resolverDir, "resolv.conf"), resolver, 0o644); err != nil {
		return nil, xerrors.Errorf("write netns resolver: %w", err)
	}
	return netns, nil
}

func (n *networkNamespace) execArgs(args []string) []string {
	result := []string{n.ipPath, "netns", "exec", n.name}
	return append(result, args...)
}

func (n *networkNamespace) Close() error {
	if n == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), reportTimeout)
	defer cancel()
	_ = runIP(ctx, n.ipPath, "netns", "del", n.name)
	_ = runIP(ctx, n.ipPath, "link", "del", n.hostVeth)
	return os.RemoveAll(filepath.Join("/etc/netns", n.name))
}

func runIP(ctx context.Context, ipPath string, args ...string) error {
	//nolint:gosec,gocritic // The namespace manager must invoke the external ip binary before the child agent exists.
	output, err := exec.CommandContext(ctx, ipPath, args...).CombinedOutput()
	if err != nil {
		return xerrors.Errorf("ip %v: %w: %s", args, err, output)
	}
	return nil
}

func randomHex(bytes int) (string, error) {
	value := make([]byte, bytes)
	if _, err := rand.Read(value); err != nil {
		return "", xerrors.Errorf("generate netns suffix: %w", err)
	}
	return hex.EncodeToString(value), nil
}
