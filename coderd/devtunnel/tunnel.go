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
)

const (
	EndpointWireguard = "wg-tunnel-udp.coder.app"
	EndpointHTTPS     = "wg-tunnel.coder.app"

	ServerPublicKey = "+KNSMwed/IlqoesvTMSBNsHFaKVLrmmaCkn0bxIhUg0="
	ServerUUID      = "fcad0000-0000-4000-8000-000000000001"
)

type Tunnel struct {
	URL      string
	Listener net.Listener
}

type Config struct {
	ID         uuid.UUID              `json:"id"`
	PrivateKey device.NoisePrivateKey `json:"private_key"`
	PublicKey  device.NoisePublicKey  `json:"public_key"`
}
type configExt struct {
	ID         uuid.UUID              `json:"id"`
	PrivateKey device.NoisePrivateKey `json:"-"`
	PublicKey  device.NoisePublicKey  `json:"public_key"`
}

// NewWithConfig calls New with the given config. For documentation, see New.
func NewWithConfig(ctx context.Context, logger slog.Logger, cfg Config) (*Tunnel, <-chan error, error) {
	err := startUpdateRoutine(ctx, logger, cfg)
	if err != nil {
		return nil, nil, xerrors.Errorf("start update routine: %w", err)
	}

	tun, tnet, err := netstack.CreateNetTUN(
		[]netip.Addr{netip.AddrFrom16(cfg.ID)},
		[]netip.Addr{netip.AddrFrom4([4]byte{1, 1, 1, 1})},
		1420,
	)
	if err != nil {
		return nil, nil, xerrors.Errorf("create net TUN: %w", err)
	}

	wgip, err := net.ResolveIPAddr("ip", EndpointWireguard)
	if err != nil {
		return nil, nil, xerrors.Errorf("resolve endpoint: %w", err)
	}

	dev := device.NewDevice(tun, conn.NewDefaultBind(), device.NewLogger(device.LogLevelVerbose, ""))
	err = dev.IpcSet(fmt.Sprintf(`private_key=%s
public_key=%s
endpoint=%s:55555
persistent_keepalive_interval=21
allowed_ip=%s/128`,
		hex.EncodeToString(cfg.PrivateKey[:]),
		encodeBase64ToHex(ServerPublicKey),
		wgip.IP.String(),
		netip.AddrFrom16(uuid.MustParse(ServerUUID)).String(),
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
			close(ch)

		case <-dev.Wait():
			close(ch)
		}
	}()

	return &Tunnel{
		URL:      fmt.Sprintf("https://%s.%s", cfg.ID, EndpointHTTPS),
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

func startUpdateRoutine(ctx context.Context, logger slog.Logger, cfg Config) error {
	// Ensure we send the first config before spawning in the background.
	_, err := sendConfigToServer(ctx, cfg)
	if err != nil {
		return xerrors.Errorf("send config to server: %w", err)
	}

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				break
			case <-ticker.C:
			}

			_, err := sendConfigToServer(ctx, cfg)
			if err != nil {
				logger.Debug(ctx, "send tunnel config to server", slog.Error(err))
			}
		}
	}()
	return nil
}

func sendConfigToServer(_ context.Context, cfg Config) (created bool, err error) {
	raw, err := json.Marshal(configExt(cfg))
	if err != nil {
		return false, xerrors.Errorf("marshal config: %w", err)
	}

	res, err := http.Post("https://"+EndpointHTTPS+"/tun", "application/json", bytes.NewReader(raw))
	if err != nil {
		return false, xerrors.Errorf("send request: %w", err)
	}
	_, _ = io.Copy(io.Discard, res.Body)
	_ = res.Body.Close()

	return res.StatusCode == http.StatusCreated, nil
}

func cfgPath() (string, error) {
	cfgDir, err := os.UserConfigDir()
	if err != nil {
		return "", xerrors.Errorf("get user config dir: %w", err)
	}

	return filepath.Join(cfgDir, "coderv2", "devtunnel"), nil
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

	return cfg, nil
}

func GenerateConfig() (Config, error) {
	priv, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return Config{}, xerrors.Errorf("generate private key: %w", err)
	}

	pub := priv.PublicKey()

	return Config{
		ID:         newUUID(),
		PrivateKey: device.NoisePrivateKey(priv),
		PublicKey:  device.NoisePublicKey(pub),
	}, nil
}

func newUUID() uuid.UUID {
	u := uuid.New()
	// 0xfc is the IPV6 prefix for internal networks.
	u[0] = 0xfc
	u[1] = 0xca

	return u
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
