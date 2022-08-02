module github.com/coder/coder

go 1.18

// Required until https://github.com/manifoldco/promptui/pull/169 is merged.
replace github.com/manifoldco/promptui => github.com/kylecarbs/promptui v0.8.1-0.20201231190244-d8f2159af2b2

// Required until https://github.com/hashicorp/terraform-config-inspect/pull/74 is merged.
replace github.com/hashicorp/terraform-config-inspect => github.com/kylecarbs/terraform-config-inspect v0.0.0-20211215004401-bbc517866b88

// Required until https://github.com/chzyer/readline/pull/198 is merged.
replace github.com/chzyer/readline => github.com/kylecarbs/readline v0.0.0-20220211054233-0d62993714c8

// Required until https://github.com/briandowns/spinner/pull/136 is merged.
replace github.com/briandowns/spinner => github.com/kylecarbs/spinner v1.18.2-0.20220329160715-20702b5af89e

// Required until https://github.com/fergusstrange/embedded-postgres/pull/75 is merged.
replace github.com/fergusstrange/embedded-postgres => github.com/kylecarbs/embedded-postgres v1.17.1-0.20220615202325-461532cecd3a

// opencensus-go leaks a goroutine by default.
replace go.opencensus.io => github.com/kylecarbs/opencensus-go v0.23.1-0.20220307014935-4d0325a68f8b

// These are to allow embedding the cloudflared quick-tunnel CLI.
// Required until https://github.com/cloudflare/cloudflared/pull/597 is merged.
replace github.com/cloudflare/cloudflared => github.com/kylecarbs/cloudflared v0.0.0-20220323202451-083379ce31c3

replace github.com/urfave/cli/v2 => github.com/ipostelnik/cli/v2 v2.3.1-0.20210324024421-b6ea8234fe3d

replace github.com/rivo/tview => github.com/kylecarbs/tview v0.0.0-20220309202238-8464256e10a1

// glog has a single goroutine leak on start that we removed in a fork: https://github.com/coder/glog/pull/1.
replace github.com/golang/glog => github.com/coder/glog v1.0.1-0.20220322161911-7365fe7f2cd1

// kcp-go starts a goroutine in an init function that we can't stop. It was
// fixed in our fork:
// https://github.com/coder/kcp-go/commit/83c0904cec69dcf21ec10c54ea666bda18ada831
replace github.com/fatedier/kcp-go => github.com/coder/kcp-go v2.0.4-0.20220409183554-83c0904cec69+incompatible

replace golang.zx2c4.com/wireguard/tun/netstack => github.com/coder/wireguard-go/tun/netstack v0.0.0-20220614153727-d82b4ba8619f

require (
	cdr.dev/slog v1.4.2-0.20220525200111-18dce5c2cd5f
	cloud.google.com/go/compute v1.7.0
	github.com/AlecAivazis/survey/v2 v2.3.5
	github.com/andybalholm/brotli v1.0.4
	github.com/armon/circbuf v0.0.0-20190214190532-5111143e8da2
	github.com/awalterschulze/gographviz v2.0.3+incompatible
	github.com/bgentry/speakeasy v0.1.0
	github.com/bramvdbogaerde/go-scp v1.2.0
	github.com/briandowns/spinner v1.18.1
	github.com/charmbracelet/charm v0.8.6
	github.com/charmbracelet/lipgloss v0.5.0
	github.com/cli/safeexec v1.0.0
	github.com/coder/retry v1.3.0
	github.com/coreos/go-oidc/v3 v3.2.0
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf
	github.com/creack/pty v1.1.18
	github.com/elastic/go-sysinfo v1.8.1
	github.com/fatih/color v1.13.0
	github.com/fatih/structs v1.1.0
	github.com/fergusstrange/embedded-postgres v1.16.0
	github.com/fullsailor/pkcs7 v0.0.0-20190404230743-d7302db945fa
	github.com/gen2brain/beeep v0.0.0-20220402123239-6a3042f4b71a
	github.com/gliderlabs/ssh v0.3.4
	github.com/go-chi/chi/v5 v5.0.7
	github.com/go-chi/httprate v0.6.0
	github.com/go-chi/render v1.0.1
	github.com/go-ping/ping v1.1.0
	github.com/go-playground/validator/v10 v10.11.0
	github.com/gofrs/flock v0.8.1
	github.com/gohugoio/hugo v0.101.0
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/golang-migrate/migrate/v4 v4.15.2
	github.com/google/go-github/v43 v43.0.1-0.20220414155304-00e42332e405
	github.com/google/uuid v1.3.0
	github.com/hashicorp/go-reap v0.0.0-20170704170343-bf58d8a43e7b
	github.com/hashicorp/go-version v1.6.0
	github.com/hashicorp/hc-install v0.4.0
	github.com/hashicorp/hcl/v2 v2.13.0
	github.com/hashicorp/terraform-config-inspect v0.0.0-20211115214459-90acf1ca460f
	github.com/hashicorp/terraform-json v0.14.0
	github.com/hashicorp/yamux v0.0.0-20220718163420-dd80a7ee44ce
	github.com/jedib0t/go-pretty/v6 v6.3.5
	github.com/justinas/nosurf v1.1.1
	github.com/kirsle/configdir v0.0.0-20170128060238-e45d2f54772f
	github.com/lib/pq v1.10.6
	github.com/mattn/go-isatty v0.0.14
	github.com/mitchellh/go-wordwrap v1.0.1
	github.com/mitchellh/mapstructure v1.5.0
	github.com/moby/moby v20.10.17+incompatible
	github.com/open-policy-agent/opa v0.43.0
	github.com/ory/dockertest/v3 v3.9.1
	github.com/pion/datachannel v1.5.2
	github.com/pion/logging v0.2.2
	github.com/pion/transport v0.13.1
	github.com/pion/turn/v2 v2.0.8
	github.com/pion/udp v0.1.1
	github.com/pion/webrtc/v3 v3.1.43
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/pkg/diff v0.0.0-20210226163009-20ebb0f2a09e
	github.com/pkg/sftp v1.13.5
	github.com/prometheus/client_golang v1.12.2
	github.com/quasilyte/go-ruleguard/dsl v0.3.21
	github.com/robfig/cron/v3 v3.0.1
	github.com/spf13/cobra v1.5.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.8.0
	github.com/tabbed/pqtype v0.1.1
	github.com/unrolled/secure v1.12.0
	go.mozilla.org/pkcs7 v0.0.0-20200128120323-432b2356ecb1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.8.0
	go.uber.org/atomic v1.9.0
	go.uber.org/goleak v1.1.12
	golang.org/x/crypto v0.0.0-20220525230936-793ad666bf5e
	golang.org/x/exp v0.0.0-20220414153411-bcd21879b8fd
	golang.org/x/mod v0.6.0-dev.0.20220419223038-86c51ed26bb4
	golang.org/x/net v0.0.0-20220630215102-69896b714898
	golang.org/x/oauth2 v0.0.0-20220622183110-fd043fe589d2
	golang.org/x/sync v0.0.0-20220601150217-0de741cfad7f
	golang.org/x/sys v0.0.0-20220715151400-c0bba94af5f8
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211
	golang.org/x/text v0.3.7
	golang.org/x/tools v0.1.11
	golang.org/x/xerrors v0.0.0-20220609144429-65e65417b02f
	golang.zx2c4.com/wireguard v0.0.0-20220703234212-c31a7b1ab478
	golang.zx2c4.com/wireguard/tun/netstack v0.0.0-00010101000000-000000000000
	golang.zx2c4.com/wireguard/wgctrl v0.0.0-20220504211119-3d4a969bb56b
	google.golang.org/api v0.90.0
	google.golang.org/protobuf v1.28.0
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/yaml.v3 v3.0.1
	inet.af/netaddr v0.0.0-20220617031823-097006376321
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	nhooyr.io/websocket v1.8.7
	storj.io/drpc v0.0.33-0.20220622181519-9206537a4db7
	tailscale.com v1.26.2
)

require (
	github.com/googleapis/enterprise-certificate-proxy v0.1.0 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
)

require (
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/OneOfOne/xxhash v1.2.8 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/agnivade/levenshtein v1.0.1 // indirect
	github.com/akutz/memconn v0.1.0 // indirect
	github.com/alecthomas/chroma v0.10.0 // indirect
	github.com/alexbrainman/sspi v0.0.0-20210105120005-909beea2cc74 // indirect
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bep/godartsass v0.14.0 // indirect
	github.com/bep/golibsass v1.1.0 // indirect
	github.com/cenkalti/backoff/v4 v4.1.3 // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/charmbracelet/bubbles v0.10.3 // indirect
	github.com/charmbracelet/bubbletea v0.20.0 // indirect
	github.com/clbanning/mxj/v2 v2.5.6 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/containerd/continuity v0.3.0 // indirect
	github.com/coreos/go-iptables v0.6.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dlclark/regexp2 v1.4.0 // indirect
	github.com/docker/cli v20.10.17+incompatible // indirect
	github.com/docker/docker v20.10.17+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/elastic/go-windows v1.0.0 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/gin-gonic/gin v1.7.0 // indirect
	github.com/go-chi/chi v1.5.4
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-ole/go-ole v1.2.6 // indirect
	github.com/go-playground/locales v0.14.0 // indirect
	github.com/go-playground/universal-translator v0.18.0 // indirect
	github.com/go-toast/toast v0.0.0-20190211030409-01e6764cf0a4 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gobwas/ws v1.1.0 // indirect
	github.com/godbus/dbus/v5 v5.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/btree v1.0.1 // indirect
	github.com/google/go-cmp v0.5.8 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.7.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/insomniacslk/dhcp v0.0.0-20211209223715-7d93572ebe8e // indirect
	github.com/joeshaw/multierror v0.0.0-20140124173710-69b34d4ec901 // indirect
	github.com/josharian/native v1.0.0 // indirect
	github.com/jsimonetti/rtnetlink v1.1.2-0.20220408201609-d380b505068b // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/klauspost/compress v1.15.9
	github.com/kortschak/wol v0.0.0-20200729010619-da482cc4850a // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/leodido/go-urn v1.2.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mdlayher/genetlink v1.2.0 // indirect
	github.com/mdlayher/netlink v1.6.0 // indirect
	github.com/mdlayher/sdnotify v1.0.0 // indirect
	github.com/mdlayher/socket v0.2.3 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b // indirect
	github.com/miekg/dns v1.1.45 // indirect
	github.com/mitchellh/go-ps v1.0.0 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/muesli/ansi v0.0.0-20211031195517-c9f0611b6c70 // indirect
	github.com/muesli/reflow v0.3.0 // indirect
	github.com/muesli/termenv v0.11.1-0.20220212125758-44cd13922739 // indirect
	github.com/nhatthm/otelsql v0.4.0
	github.com/niklasfasching/go-org v1.6.5 // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.3-0.20220114050600-8b9d41f48198 // indirect
	github.com/opencontainers/runc v1.1.2 // indirect
	github.com/pelletier/go-toml/v2 v2.0.2 // indirect
	github.com/pion/dtls/v2 v2.1.5 // indirect
	github.com/pion/ice/v2 v2.2.6 // indirect
	github.com/pion/interceptor v0.1.11 // indirect
	github.com/pion/mdns v0.0.5 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.9 // indirect
	github.com/pion/rtp v1.7.13 // indirect
	github.com/pion/sctp v1.8.2 // indirect
	github.com/pion/sdp/v3 v3.0.5 // indirect
	github.com/pion/srtp/v2 v2.0.10 // indirect
	github.com/pion/stun v0.3.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/spf13/afero v1.9.2
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/tadvi/systray v0.0.0-20190226123456-11a2b8fa57af // indirect
	github.com/tailscale/certstore v0.1.1-0.20220316223106-78d6e1c49d8d // indirect
	github.com/tailscale/golang-x-crypto v0.0.0-20220428210705-0b941c09a5e1 // indirect
	github.com/tailscale/goupnp v1.0.1-0.20210804011211-c64d0f06ea05 // indirect
	github.com/tailscale/netlink v1.1.1-0.20211101221916-cabfb018fe85 // indirect
	github.com/tcnksm/go-httpstat v0.2.0 // indirect
	github.com/tdewolff/parse/v2 v2.6.0 // indirect
	github.com/u-root/uio v0.0.0-20210528151154-e40b768296a7 // indirect
	github.com/vektah/gqlparser/v2 v2.4.6 // indirect
	github.com/vishvananda/netlink v1.1.1-0.20211118161826-650dca95af54 // indirect
	github.com/vishvananda/netns v0.0.0-20211101163701-50045581ed74 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/yashtewari/glob-intersection v0.1.0 // indirect
	github.com/yuin/goldmark v1.4.12 // indirect
	github.com/zclconf/go-cty v1.10.0 // indirect
	github.com/zeebo/errs v1.3.0 // indirect
	go.opencensus.io v0.23.0 // indirect
	go.opentelemetry.io/otel v1.8.0
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.8.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.7.0
	go.opentelemetry.io/otel/metric v0.31.0 // indirect
	go.opentelemetry.io/otel/sdk v1.8.0
	go.opentelemetry.io/otel/trace v1.8.0
	go.opentelemetry.io/proto/otlp v0.18.0 // indirect
	go4.org/intern v0.0.0-20211027215823-ae77deb06f29 // indirect
	go4.org/mem v0.0.0-20210711025021-927187094b94 // indirect
	go4.org/unsafe/assume-no-moving-gc v0.0.0-20220617031537-928513b29760 // indirect
	golang.org/x/time v0.0.0-20220224211638-0e9765cccd65 // indirect
	golang.zx2c4.com/wintun v0.0.0-20211104114900-415007cec224 // indirect
	golang.zx2c4.com/wireguard/windows v0.4.10 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220624142145-8cd45d7dbd1f // indirect
	google.golang.org/grpc v1.48.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gvisor.dev/gvisor v0.0.0-20220407223209-21871174d445 // indirect
	howett.net/plist v1.0.0 // indirect
)
