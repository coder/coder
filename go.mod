module github.com/coder/coder

go 1.19

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

replace github.com/urfave/cli/v2 => github.com/ipostelnik/cli/v2 v2.3.1-0.20210324024421-b6ea8234fe3d

replace github.com/rivo/tview => github.com/kylecarbs/tview v0.0.0-20220309202238-8464256e10a1

// glog has a single goroutine leak on start that we removed in a fork: https://github.com/coder/glog/pull/1.
replace github.com/golang/glog => github.com/coder/glog v1.0.1-0.20220322161911-7365fe7f2cd1

// kcp-go starts a goroutine in an init function that we can't stop. It was
// fixed in our fork:
// https://github.com/coder/kcp-go/commit/83c0904cec69dcf21ec10c54ea666bda18ada831
replace github.com/fatedier/kcp-go => github.com/coder/kcp-go v2.0.4-0.20220409183554-83c0904cec69+incompatible

// https://github.com/pion/udp/pull/73
replace github.com/pion/udp => github.com/mafredri/udp v0.1.2-0.20220805105907-b2872e92e98d

// https://github.com/tcnksm/go-httpstat/pull/29
replace github.com/tcnksm/go-httpstat => github.com/kylecarbs/go-httpstat v0.0.0-20220831233600-c91452099472

// There are a few minor changes we make to Tailscale that we're slowly upstreaming. Compare here:
// https://github.com/tailscale/tailscale/compare/main...coder:tailscale:main
replace tailscale.com => github.com/coder/tailscale v1.1.1-0.20221113171243-7d90f070c5dc

// Switch to our fork that imports fixes from http://github.com/tailscale/ssh.
// See: https://github.com/coder/coder/issues/3371
//
// Note that http://github.com/tailscale/ssh has been merged into the Tailscale
// repo as tailscale.com/tempfork/gliderlabs/ssh, however, we can't replace the
// subpath and it includes changes to golang.org/x/crypto/ssh as well which
// makes importing it directly a bit messy.
replace github.com/gliderlabs/ssh => github.com/coder/ssh v0.0.0-20220811105153-fcea99919338

require (
	cdr.dev/slog v1.4.2-0.20220525200111-18dce5c2cd5f
	cloud.google.com/go/compute v1.12.1 // indirect
	cloud.google.com/go/compute/metadata v0.2.1
	github.com/AlecAivazis/survey/v2 v2.3.5
	github.com/adrg/xdg v0.4.0
	github.com/andybalholm/brotli v1.0.4
	github.com/armon/circbuf v0.0.0-20190214190532-5111143e8da2
	github.com/awalterschulze/gographviz v2.0.3+incompatible
	github.com/bgentry/speakeasy v0.1.0
	github.com/bramvdbogaerde/go-scp v1.2.0
	github.com/briandowns/spinner v1.18.1
	github.com/cakturk/go-netstat v0.0.0-20200220111822-e5b49efee7a5
	github.com/charmbracelet/charm v0.12.1
	github.com/charmbracelet/lipgloss v0.6.0
	github.com/cli/safeexec v1.0.0
	github.com/coder/retry v1.3.0
	github.com/coreos/go-oidc/v3 v3.4.0
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf
	github.com/creack/pty v1.1.18
	github.com/elastic/go-sysinfo v1.8.1
	github.com/fatih/color v1.13.0
	github.com/fatih/structs v1.1.0
	github.com/fatih/structtag v1.2.0
	github.com/fergusstrange/embedded-postgres v1.16.0
	github.com/fullsailor/pkcs7 v0.0.0-20190404230743-d7302db945fa
	github.com/gen2brain/beeep v0.0.0-20220402123239-6a3042f4b71a
	github.com/gliderlabs/ssh v0.3.4
	github.com/go-chi/chi v1.5.4
	github.com/go-chi/chi/v5 v5.0.7
	github.com/go-chi/httprate v0.7.0
	github.com/go-chi/render v1.0.1
	github.com/go-logr/logr v1.2.3
	github.com/go-ping/ping v1.1.0
	github.com/go-playground/validator/v10 v10.11.0
	github.com/gofrs/flock v0.8.1
	github.com/gohugoio/hugo v0.105.0
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/golang-jwt/jwt/v4 v4.4.2
	github.com/golang-migrate/migrate/v4 v4.15.2
	github.com/golang/protobuf v1.5.2
	github.com/google/go-github/v43 v43.0.1-0.20220414155304-00e42332e405
	github.com/google/uuid v1.3.0
	github.com/hashicorp/go-reap v0.0.0-20170704170343-bf58d8a43e7b
	github.com/hashicorp/go-version v1.6.0
	github.com/hashicorp/hc-install v0.4.1-0.20220912074615-4487b02cbcbb
	github.com/hashicorp/hcl/v2 v2.14.0
	github.com/hashicorp/terraform-config-inspect v0.0.0-20211115214459-90acf1ca460f
	github.com/hashicorp/terraform-json v0.14.0
	github.com/hashicorp/yamux v0.0.0-20220718163420-dd80a7ee44ce
	github.com/imulab/go-scim/pkg/v2 v2.2.0
	github.com/jedib0t/go-pretty/v6 v6.4.0
	github.com/jmoiron/sqlx v1.3.5
	github.com/justinas/nosurf v1.1.1
	github.com/kirsle/configdir v0.0.0-20170128060238-e45d2f54772f
	github.com/klauspost/compress v1.15.9
	github.com/lib/pq v1.10.6
	github.com/mattn/go-isatty v0.0.16
	github.com/mitchellh/go-wordwrap v1.0.1
	github.com/mitchellh/mapstructure v1.5.0
	github.com/moby/moby v20.10.21+incompatible
	github.com/open-policy-agent/opa v0.44.0
	github.com/ory/dockertest/v3 v3.9.1
	github.com/pion/udp v0.1.1
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/pkg/diff v0.0.0-20210226163009-20ebb0f2a09e
	github.com/pkg/sftp v1.13.6-0.20221018182125-7da137aa03f0
	github.com/prometheus/client_golang v1.13.0
	github.com/quasilyte/go-ruleguard/dsl v0.3.21
	github.com/robfig/cron/v3 v3.0.1
	github.com/spf13/afero v1.9.2
	github.com/spf13/cobra v1.6.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.13.0
	github.com/stretchr/testify v1.8.0
	github.com/tabbed/pqtype v0.1.1
	github.com/u-root/u-root v0.10.0
	github.com/unrolled/secure v1.13.0
	go.mozilla.org/pkcs7 v0.0.0-20200128120323-432b2356ecb1
	go.nhat.io/otelsql v0.7.0
	go.opentelemetry.io/otel v1.11.1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.11.1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc v1.11.1
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.11.1
	go.opentelemetry.io/otel/sdk v1.11.1
	go.opentelemetry.io/otel/trace v1.11.1
	go.uber.org/atomic v1.10.0
	go.uber.org/goleak v1.2.0
	go4.org/netipx v0.0.0-20220725152314-7e7bdc8411bf
	golang.org/x/crypto v0.1.0
	golang.org/x/exp v0.0.0-20220722155223-a9213eeb770e
	golang.org/x/mod v0.6.0
	golang.org/x/oauth2 v0.0.0-20221014153046-6fdb5e3db783
	golang.org/x/sync v0.0.0-20220929204114-8fcdb60fdcc0
	golang.org/x/sys v0.1.0
	golang.org/x/term v0.1.0
	golang.org/x/text v0.4.0
	golang.org/x/tools v0.2.0
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2
	golang.zx2c4.com/wireguard v0.0.0-20220920152132-bb719d3a6e2c
	golang.zx2c4.com/wireguard/wgctrl v0.0.0-20220504211119-3d4a969bb56b
	google.golang.org/api v0.100.0
	google.golang.org/grpc v1.50.1
	google.golang.org/protobuf v1.28.1
	gopkg.in/natefinch/lumberjack.v2 v2.0.0
	gopkg.in/square/go-jose.v2 v2.6.0
	gopkg.in/yaml.v3 v3.0.1
	gvisor.dev/gvisor v0.0.0-20220817001344-846276b3dbc5
	k8s.io/utils v0.0.0-20220210201930-3a6ce19ff2f9
	nhooyr.io/websocket v1.8.7
	storj.io/drpc v0.0.33-0.20220622181519-9206537a4db7
	tailscale.com v1.32.2
)

require (
	filippo.io/edwards25519 v1.0.0-rc.1 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/OneOfOne/xxhash v1.2.8 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/agnivade/levenshtein v1.1.1 // indirect
	github.com/akutz/memconn v0.1.0 // indirect
	github.com/alecthomas/chroma v0.10.0 // indirect
	github.com/alexbrainman/sspi v0.0.0-20210105120005-909beea2cc74 // indirect
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bep/godartsass v0.14.0 // indirect
	github.com/bep/golibsass v1.1.0 // indirect
	github.com/cenkalti/backoff/v4 v4.1.3
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
	github.com/fsnotify/fsnotify v1.6.0 // indirect
	github.com/fxamacker/cbor/v2 v2.4.0 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/gin-gonic/gin v1.7.0 // indirect
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
	github.com/google/btree v1.0.1 // indirect
	github.com/google/go-cmp v0.5.9 // indirect
	github.com/google/go-querystring v1.1.0 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.7.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hdevalence/ed25519consensus v0.0.0-20220222234857-c00d1f31bab3 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.0.1 // indirect
	github.com/insomniacslk/dhcp v0.0.0-20211209223715-7d93572ebe8e // indirect
	github.com/joeshaw/multierror v0.0.0-20140124173710-69b34d4ec901 // indirect
	github.com/josharian/native v1.0.0 // indirect
	github.com/jsimonetti/rtnetlink v1.1.2-0.20220408201609-d380b505068b // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/kortschak/wol v0.0.0-20200729010619-da482cc4850a // indirect
	github.com/kr/fs v0.1.0 // indirect
	github.com/leodido/go-urn v1.2.1 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/magiconair/properties v1.8.6 // indirect
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
	github.com/niklasfasching/go-org v1.6.5 // indirect
	github.com/nu7hatch/gouuid v0.0.0-20131221200532-179d4d0c4d8d // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.3-0.20220114050600-8b9d41f48198 // indirect
	github.com/opencontainers/runc v1.1.2 // indirect
	github.com/pelletier/go-toml v1.9.5 // indirect
	github.com/pelletier/go-toml/v2 v2.0.5 // indirect
	github.com/pion/transport v0.13.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.37.0 // indirect
	github.com/prometheus/procfs v0.8.0 // indirect
	github.com/rcrowley/go-metrics v0.0.0-20200313005456-10cdbea86bc0 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/sirupsen/logrus v1.9.0 // indirect
	github.com/spf13/cast v1.5.0 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/subosito/gotenv v1.4.1 // indirect
	github.com/tadvi/systray v0.0.0-20190226123456-11a2b8fa57af // indirect
	github.com/tailscale/certstore v0.1.1-0.20220316223106-78d6e1c49d8d // indirect
	github.com/tailscale/golang-x-crypto v0.0.0-20221102133106-bc99ab8c2d17 // indirect
	github.com/tailscale/goupnp v1.0.1-0.20210804011211-c64d0f06ea05 // indirect
	github.com/tailscale/netlink v1.1.1-0.20211101221916-cabfb018fe85 // indirect
	github.com/tchap/go-patricia/v2 v2.3.1 // indirect
	github.com/tcnksm/go-httpstat v0.2.0 // indirect
	github.com/tdewolff/parse/v2 v2.6.4 // indirect
	github.com/u-root/uio v0.0.0-20220204230159-dac05f7d2cb4 // indirect
	github.com/vishvananda/netlink v1.1.1-0.20211118161826-650dca95af54 // indirect
	github.com/vishvananda/netns v0.0.0-20211101163701-50045581ed74 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/xi2/xz v0.0.0-20171230120015-48954b6210f8 // indirect
	github.com/yashtewari/glob-intersection v0.1.0 // indirect
	github.com/yuin/goldmark v1.5.2 // indirect
	github.com/zclconf/go-cty v1.10.0 // indirect
	github.com/zeebo/errs v1.3.0 // indirect
	go.opencensus.io v0.23.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/internal/retry v1.11.1 // indirect
	go.opentelemetry.io/otel/metric v0.33.0 // indirect
	go.opentelemetry.io/proto/otlp v0.19.0 // indirect
	go4.org/mem v0.0.0-20210711025021-927187094b94 // indirect
	golang.org/x/net v0.1.0 // indirect
	golang.org/x/time v0.0.0-20220224211638-0e9765cccd65 // indirect
	golang.zx2c4.com/wintun v0.0.0-20211104114900-415007cec224 // indirect
	golang.zx2c4.com/wireguard/windows v0.5.3 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20221024183307-1bc688fe9f3e // indirect
	gopkg.in/ini.v1 v1.67.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	howett.net/plist v1.0.0 // indirect
)
