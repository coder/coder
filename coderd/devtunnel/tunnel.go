package devtunnel

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"golang.org/x/xerrors"
	"golang.zx2c4.com/wireguard/conn"
	"golang.zx2c4.com/wireguard/device"
	"golang.zx2c4.com/wireguard/tun/netstack"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"

	"cdr.dev/slog"
	"github.com/coder/coder/cryptorand"
)

var (
	v0EndpointHTTPS = "wg-tunnel.coder.app"

	v0ServerPublicKey = "+KNSMwed/IlqoesvTMSBNsHFaKVLrmmaCkn0bxIhUg0="
	v0ServerIP        = netip.AddrFrom16(uuid.MustParse("fcad0000-0000-4000-8000-000000000001"))
)

type Tunnel struct {
	URL      string
	Listener net.Listener
}

type Config struct {
	Version    int                    `json:"version"`
	ID         uuid.UUID              `json:"id"`
	PrivateKey device.NoisePrivateKey `json:"private_key"`
	PublicKey  device.NoisePublicKey  `json:"public_key"`

	Tunnel Node `json:"tunnel"`
}
type configExt struct {
	Version    int                    `json:"-"`
	ID         uuid.UUID              `json:"id"`
	PrivateKey device.NoisePrivateKey `json:"-"`
	PublicKey  device.NoisePublicKey  `json:"public_key"`

	Tunnel Node `json:"-"`
}

// NewWithConfig calls New with the given config. For documentation, see New.
func NewWithConfig(ctx context.Context, logger slog.Logger, cfg Config) (*Tunnel, <-chan error, error) {
	server, routineEnd, err := startUpdateRoutine(ctx, logger, cfg)
	if err != nil {
		return nil, nil, xerrors.Errorf("start update routine: %w", err)
	}

	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{server.ClientIP},
		[]netip.Addr{netip.AddrFrom4([4]byte{1, 1, 1, 1})},
		1280,
	)
	if err != nil {
		return nil, nil, xerrors.Errorf("create net TUN: %w", err)
	}

	wgip, err := net.ResolveIPAddr("ip", cfg.Tunnel.HostnameWireguard)
	if err != nil {
		return nil, nil, xerrors.Errorf("resolve endpoint: %w", err)
	}

	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelSilent, ""))
	err = dev.IpcSet(fmt.Sprintf(`private_key=%s
public_key=%s
endpoint=%s:%d
persistent_keepalive_interval=21
allowed_ip=%s/128`,
		hex.EncodeToString(cfg.PrivateKey[:]),
		server.ServerPublicKey,
		wgip.IP.String(),
		cfg.Tunnel.WireguardPort,
		server.ServerIP.String(),
	))
	if err != nil {
		return nil, nil, xerrors.Errorf("configure wireguard ipc: %w", err)
	}

	err = dev.Up()
	if err != nil {
		return nil, nil, xerrors.Errorf("wireguard device up: %w", err)
	}

	wgListen, err := tnet.ListenTCP(&net.TCPAddr{Port: 8090})
	if err != nil {
		return nil, nil, xerrors.Errorf("wireguard device listen: %w", err)
	}

	ch := make(chan error)
	go func() {
		select {
		case <-ctx.Done():
			_ = wgListen.Close()
			dev.Close()
			<-routineEnd
			close(ch)

		case <-dev.Wait():
			close(ch)
		}
	}()

	return &Tunnel{
		URL:      fmt.Sprintf("https://%s", server.Hostname),
		Listener: wgListen,
	}, ch, nil
}

// New creates a tunnel with a public URL and returns a listener for incoming
// connections on that URL. Connections are made over the wireguard protocol.
// Tunnel configuration is cached in the user's config directory. Successive
// calls to New will always use the same URL. If multiple public URLs in
// parallel are required, use NewWithConfig.
func New(ctx context.Context, logger slog.Logger) (*Tunnel, <-chan error, error) {
	cfg, err := readOrGenerateConfig()
	if err != nil {
		return nil, nil, xerrors.Errorf("read or generate config: %w", err)
	}

	return NewWithConfig(ctx, logger, cfg)
}

func startUpdateRoutine(ctx context.Context, logger slog.Logger, cfg Config) (ServerResponse, <-chan struct{}, error) {
	// Ensure we send the first config before spawning in the background.
	res, err := sendConfigToServer(ctx, cfg)
	if err != nil {
		return ServerResponse{}, nil, xerrors.Errorf("send config to server: %w", err)
	}

	endCh := make(chan struct{})
	go func() {
		defer close(endCh)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return

			case <-ticker.C:
			}

			_, err := sendConfigToServer(ctx, cfg)
			if err != nil {
				logger.Debug(ctx, "send tunnel config to server", slog.Error(err))
			}
		}
	}()
	return res, endCh, nil
}

type ServerResponse struct {
	Hostname        string     `json:"hostname"`
	ServerIP        netip.Addr `json:"server_ip"`
	ServerPublicKey string     `json:"server_public_key"` // hex
	ClientIP        netip.Addr `json:"client_ip"`
}

func sendConfigToServer(ctx context.Context, cfg Config) (ServerResponse, error) {
	raw, err := json.Marshal(configExt(cfg))
	if err != nil {
		return ServerResponse{}, xerrors.Errorf("marshal config: %w", err)
	}

	var req *http.Request
	switch cfg.Version {
	case 0:
		req, err = http.NewRequestWithContext(ctx, "POST", "https://"+v0EndpointHTTPS+"/tun", bytes.NewReader(raw))
		if err != nil {
			return ServerResponse{}, xerrors.Errorf("new request: %w", err)
		}

	case 1:
		req, err = http.NewRequestWithContext(ctx, "POST", "https://"+cfg.Tunnel.HostnameHTTPS+"/tun", bytes.NewReader(raw))
		if err != nil {
			return ServerResponse{}, xerrors.Errorf("new request: %w", err)
		}

	default:
		return ServerResponse{}, xerrors.Errorf("unknown config version: %d", cfg.Version)
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return ServerResponse{}, xerrors.Errorf("do request: %w", err)
	}
	defer res.Body.Close()

	var resp ServerResponse
	switch cfg.Version {
	case 0:
		_, _ = io.Copy(io.Discard, res.Body)
		resp.Hostname = fmt.Sprintf("%s.%s", cfg.ID, v0EndpointHTTPS)
		resp.ServerIP = v0ServerIP
		resp.ServerPublicKey = encodeBase64ToHex(v0ServerPublicKey)
		resp.ClientIP = netip.AddrFrom16(cfg.ID)

	case 1:
		err := json.NewDecoder(res.Body).Decode(&resp)
		if err != nil {
			return ServerResponse{}, xerrors.Errorf("decode response: %w", err)
		}

	default:
		_, _ = io.Copy(io.Discard, res.Body)
		return ServerResponse{}, xerrors.Errorf("unknown config version: %d", cfg.Version)
	}

	return resp, nil
}

func cfgPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", xerrors.Errorf("get user config dir: %w", err)
	}

	cfgDir = filepath.Join(cfgDir, "coderv2")
	err = os.MkdirAll(cfgDir, 0750)
	if err != nil {
		return "", xerrors.Errorf("mkdirall config dir %q: %w", cfgDir, err)
	}

	return filepath.Join(cfgDir, "devtunnel"), nil
}

func readOrGenerateConfig() (Config, error) {
	cfgFi, err := cfgPath()
	if err != nil {
		return Config{}, xerrors.Errorf("get config path: %w", err)
	}

	fi, err := os.ReadFile(cfgFi)
	if err != nil {
		if os.IsNotExist(err) {
			cfg, err := GenerateConfig()
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
		cfg.Tunnel = Node{
			ID:                0,
			HostnameHTTPS:     "wg-tunnel.coder.app",
			HostnameWireguard: "wg-tunnel-udp.coder.app",
			WireguardPort:     55555,
		}
	}

	return cfg, nil
}

func GenerateConfig() (Config, error) {
	priv, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return Config{}, xerrors.Errorf("generate private key: %w", err)
	}
	pub := priv.PublicKey()

	node, err := FindClosestNode()
	if err != nil {
		region := Regions[0]
		n, _ := cryptorand.Intn(len(region.Nodes))
		node = region.Nodes[n]
		_, _ = fmt.Println("Error picking closest dev tunnel:", err)
		_, _ = fmt.Println("Defaulting to", Regions[0].LocationName)
	}

	return Config{
		Version:    1,
		PrivateKey: device.NoisePrivateKey(priv),
		PublicKey:  device.NoisePublicKey(pub),
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

	err = os.WriteFile(cfgFi, raw, 0600)
	if err != nil {
		return xerrors.Errorf("write file: %w", err)
	}

	return nil
}

func encodeBase64ToHex(key string) string {
	decoded, err := base64.StdEncoding.DecodeString(key)
	if err != nil {
		panic(err)
	}

	if len(decoded) != 32 {
		panic((xerrors.New("key should be 32 bytes: " + key)))
	}

	return hex.EncodeToString(decoded)
}
