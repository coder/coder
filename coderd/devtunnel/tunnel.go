package devtunnel

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"github.com/briandowns/spinner"
	"github.com/tailscale/wireguard-go/device"
	"golang.org/x/xerrors"

	"cdr.dev/slog"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/pretty"
	"github.com/coder/wgtunnel/tunnelsdk"
)

type Config struct {
	Version    tunnelsdk.TunnelVersion `json:"version"`
	PrivateKey device.NoisePrivateKey  `json:"private_key"`
	PublicKey  device.NoisePublicKey   `json:"public_key"`

	Tunnel Node `json:"tunnel"`

	// Used in testing.  Normally this is nil, indicating to use DefaultClient.
	HTTPClient *http.Client `json:"-"`
}

// NewWithConfig calls New with the given config. For documentation, see New.
func NewWithConfig(ctx context.Context, logger slog.Logger, cfg Config) (*tunnelsdk.Tunnel, error) {
	u := &url.URL{
		Scheme: "https",
		Host:   cfg.Tunnel.HostnameHTTPS,
	}

	c := tunnelsdk.New(u)
	if cfg.HTTPClient != nil {
		c.HTTPClient = cfg.HTTPClient
	}
	return c.LaunchTunnel(ctx, tunnelsdk.TunnelConfig{
		Log:        logger,
		Version:    cfg.Version,
		PrivateKey: tunnelsdk.FromNoisePrivateKey(cfg.PrivateKey),
	})
}

// New creates a tunnel with a public URL and returns a listener for incoming
// connections on that URL. Connections are made over the wireguard protocol.
// Tunnel configuration is cached in the user's config directory. Successive
// calls to New will always use the same URL. If multiple public URLs in
// parallel are required, use NewWithConfig.
//
// This uses https://github.com/coder/wgtunnel as the server and client
// implementation.
func New(ctx context.Context, logger slog.Logger, customTunnelHost string) (*tunnelsdk.Tunnel, error) {
	cfg, err := readOrGenerateConfig(customTunnelHost)
	if err != nil {
		return nil, xerrors.Errorf("read or generate config: %w", err)
	}

	return NewWithConfig(ctx, logger, cfg)
}

func cfgPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", xerrors.Errorf("get user config dir: %w", err)
	}

	cfgDir = filepath.Join(cfgDir, "coderv2")
	err = os.MkdirAll(cfgDir, 0o750)
	if err != nil {
		return "", xerrors.Errorf("mkdirall config dir %q: %w", cfgDir, err)
	}

	return filepath.Join(cfgDir, "devtunnel"), nil
}

func readOrGenerateConfig(customTunnelHost string) (Config, error) {
	cfgFi, err := cfgPath()
	if err != nil {
		return Config{}, xerrors.Errorf("get config path: %w", err)
	}

	fi, err := os.ReadFile(cfgFi)
	if err != nil {
		if os.IsNotExist(err) {
			cfg, err := GenerateConfig(customTunnelHost)
			if err != nil {
				return Config{}, xerrors.Errorf("generate config: %w", err)
			}

			err = writeConfig(cfg)
			if err != nil {
				return Config{}, xerrors.Errorf("write config: %w", err)
			}

			return cfg, nil
		}

		return Config{}, xerrors.Errorf("read config: %w", err)
	}

	cfg := Config{}
	err = json.Unmarshal(fi, &cfg)
	if err != nil {
		return Config{}, xerrors.Errorf("unmarshal config: %w", err)
	}

	if cfg.Version == 0 {
		_, _ = fmt.Println()
		pretty.Printf(cliui.DefaultStyles.Error, "You're running a deprecated tunnel version.\n")
		pretty.Printf(cliui.DefaultStyles.Error, "Upgrading you to the new version now. You will need to rebuild running workspaces.")
		_, _ = fmt.Println()

		cfg, err := GenerateConfig(customTunnelHost)
		if err != nil {
			return Config{}, xerrors.Errorf("generate config: %w", err)
		}

		err = writeConfig(cfg)
		if err != nil {
			return Config{}, xerrors.Errorf("write config: %w", err)
		}

		return cfg, nil
	}

	return cfg, nil
}

func GenerateConfig(customTunnelHost string) (Config, error) {
	priv, err := tunnelsdk.GeneratePrivateKey()
	if err != nil {
		return Config{}, xerrors.Errorf("generate private key: %w", err)
	}
	privNoisePublicKey, err := priv.NoisePrivateKey()
	if err != nil {
		return Config{}, xerrors.Errorf("generate noise private key: %w", err)
	}
	pubNoisePublicKey := priv.NoisePublicKey()

	spin := spinner.New(spinner.CharSets[39], 350*time.Millisecond)
	spin.Suffix = " Finding the closest tunnel region..."
	spin.Start()

	nodes, err := Nodes(customTunnelHost)
	if err != nil {
		return Config{}, xerrors.Errorf("get nodes: %w", err)
	}
	node, err := FindClosestNode(nodes)
	if err != nil {
		// If we fail to find the closest node, default to a random node from
		// the first region.
		region := Regions[0]
		n, _ := cryptorand.Intn(len(region.Nodes))
		node = region.Nodes[n]
		spin.Stop()
		_, _ = fmt.Println("Error picking closest dev tunnel:", err)
		_, _ = fmt.Println("Defaulting to", Regions[0].LocationName)
	}

	locationName := "Unknown"
	if node.RegionID < len(Regions) {
		locationName = Regions[node.RegionID].LocationName
	}

	spin.Stop()
	_, _ = fmt.Printf("Using tunnel in %s with latency %s.\n",
		cliui.Keyword(locationName),
		cliui.Code(node.AvgLatency.String()),
	)

	return Config{
		Version:    tunnelsdk.TunnelVersion2,
		PrivateKey: privNoisePublicKey,
		PublicKey:  pubNoisePublicKey,
		Tunnel:     node,
	}, nil
}

func writeConfig(cfg Config) error {
	cfgFi, err := cfgPath()
	if err != nil {
		return xerrors.Errorf("get config path: %w", err)
	}

	raw, err := json.Marshal(cfg)
	if err != nil {
		return xerrors.Errorf("marshal config: %w", err)
	}

	err = os.WriteFile(cfgFi, raw, 0o600)
	if err != nil {
		return xerrors.Errorf("write file: %w", err)
	}

	return nil
}
