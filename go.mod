module github.com/coder/coder

go 1.18

// Required until https://github.com/manifoldco/promptui/pull/169 is merged.
replace github.com/manifoldco/promptui => github.com/kylecarbs/promptui v0.8.1-0.20201231190244-d8f2159af2b2

// Required until https://github.com/hashicorp/terraform-exec/pull/275 and https://github.com/hashicorp/terraform-exec/pull/276 are merged.
replace github.com/hashicorp/terraform-exec => github.com/kylecarbs/terraform-exec v0.15.1-0.20220202050609-a1ce7181b180

// Required until https://github.com/hashicorp/terraform-config-inspect/pull/74 is merged.
replace github.com/hashicorp/terraform-config-inspect => github.com/kylecarbs/terraform-config-inspect v0.0.0-20211215004401-bbc517866b88

// Required until https://github.com/chzyer/readline/pull/198 is merged.
replace github.com/chzyer/readline => github.com/kylecarbs/readline v0.0.0-20220211054233-0d62993714c8

// Required until https://github.com/briandowns/spinner/pull/136 is merged.
replace github.com/briandowns/spinner => github.com/kylecarbs/spinner v1.18.2-0.20220329160715-20702b5af89e

// opencensus-go leaks a goroutine by default.
replace go.opencensus.io => github.com/kylecarbs/opencensus-go v0.23.1-0.20220307014935-4d0325a68f8b

// These are to allow embedding the cloudflared quick-tunnel CLI.
// Required until https://github.com/cloudflare/cloudflared/pull/597 is merged.
replace github.com/cloudflare/cloudflared => github.com/kylecarbs/cloudflared v0.0.0-20220323202451-083379ce31c3

replace github.com/urfave/cli/v2 => github.com/ipostelnik/cli/v2 v2.3.1-0.20210324024421-b6ea8234fe3d

replace github.com/rivo/tview => github.com/kylecarbs/tview v0.0.0-20220309202238-8464256e10a1

// glog has a single goroutine leak on start that we removed in a fork: https://github.com/coder/glog/pull/1.
replace github.com/golang/glog => github.com/coder/glog v1.0.1-0.20220322161911-7365fe7f2cd1

require (
	cdr.dev/slog v1.4.1
	cloud.google.com/go/compute v1.5.0
	github.com/AlecAivazis/survey/v2 v2.3.4
	github.com/awalterschulze/gographviz v2.0.3+incompatible
	github.com/bgentry/speakeasy v0.1.0
	github.com/briandowns/spinner v1.18.1
	github.com/charmbracelet/charm v0.11.0
	github.com/charmbracelet/lipgloss v0.5.0
	github.com/cloudflare/cloudflared v0.0.0-20220308214351-5352b3cf0489
	github.com/coder/retry v1.3.0
	github.com/coreos/go-systemd v0.0.0-20191104093116-d3cd4ed1dbcf
	github.com/creack/pty v1.1.18
	github.com/fatih/color v1.13.0
	github.com/gliderlabs/ssh v0.3.3
	github.com/go-chi/chi/v5 v5.0.7
	github.com/go-chi/render v1.0.1
	github.com/go-playground/validator/v10 v10.10.1
	github.com/gohugoio/hugo v0.96.0
	github.com/golang-jwt/jwt v3.2.2+incompatible
	github.com/golang-migrate/migrate/v4 v4.15.1
	github.com/google/uuid v1.3.0
	github.com/hashicorp/go-version v1.4.0
	github.com/hashicorp/hc-install v0.3.1
	github.com/hashicorp/hcl/v2 v2.11.1
	github.com/hashicorp/terraform-config-inspect v0.0.0-20211115214459-90acf1ca460f
	github.com/hashicorp/terraform-exec v0.15.0
	github.com/hashicorp/yamux v0.0.0-20211028200310-0bc27b27de87
	github.com/justinas/nosurf v1.1.1
	github.com/kirsle/configdir v0.0.0-20170128060238-e45d2f54772f
	github.com/lib/pq v1.10.4
	github.com/manifoldco/promptui v0.9.0
	github.com/mattn/go-isatty v0.0.14
	github.com/mitchellh/mapstructure v1.4.3
	github.com/moby/moby v20.10.14+incompatible
	github.com/ory/dockertest/v3 v3.8.1
	github.com/pion/datachannel v1.5.2
	github.com/pion/logging v0.2.2
	github.com/pion/transport v0.13.0
	github.com/pion/webrtc/v3 v3.1.27
	github.com/pkg/browser v0.0.0-20210911075715-681adbf594b8
	github.com/quasilyte/go-ruleguard/dsl v0.3.19
	github.com/rs/zerolog v1.26.1
	github.com/spf13/cobra v1.4.0
	github.com/spf13/pflag v1.0.5
	github.com/stretchr/testify v1.7.1
	github.com/tabbed/pqtype v0.1.1
	github.com/unrolled/secure v1.10.0
	github.com/urfave/cli/v2 v2.3.0
	go.uber.org/atomic v1.9.0
	go.uber.org/goleak v1.1.12
	golang.org/x/crypto v0.0.0-20220315160706-3147a52a75dd
	golang.org/x/oauth2 v0.0.0-20220309155454-6242fa91716a
	golang.org/x/sync v0.0.0-20210220032951-036812b2e83c
	golang.org/x/sys v0.0.0-20220310020820-b874c991c1a5
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	google.golang.org/api v0.73.0
	google.golang.org/protobuf v1.28.0
	nhooyr.io/websocket v1.8.7
	storj.io/drpc v0.0.30
)

require github.com/cli/safeexec v1.0.0

require (
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/BurntSushi/toml v1.0.0 // indirect
	github.com/DataDog/datadog-agent/pkg/obfuscate v0.0.0-20211129110424-6491aa3bf583 // indirect
	github.com/DataDog/datadog-go v4.8.2+incompatible // indirect
	github.com/DataDog/datadog-go/v5 v5.0.2 // indirect
	github.com/DataDog/sketches-go v1.0.0 // indirect
	github.com/Microsoft/go-winio v0.5.2 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/agext/levenshtein v1.2.3 // indirect
	github.com/alecthomas/chroma v0.10.0 // indirect
	github.com/anmitsu/go-shlex v0.0.0-20200514113438-38f4b401e2be // indirect
	github.com/apparentlymart/go-cidr v1.1.0 // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.1.2 // indirect
	github.com/certifi/gocertifi v0.0.0-20210507211836-431795d63e8d // indirect
	github.com/cespare/xxhash/v2 v2.1.2 // indirect
	github.com/charmbracelet/bubbles v0.10.3 // indirect
	github.com/charmbracelet/bubbletea v0.20.0 // indirect
	github.com/cheekybits/genny v1.0.0 // indirect
	github.com/chzyer/readline v0.0.0-20180603132655-2972be24d48e // indirect
	github.com/clbanning/mxj/v2 v2.5.5 // indirect
	github.com/cloudflare/brotli-go v0.0.0-20191101163834-d34379f7ff93 // indirect
	github.com/cloudflare/golibs v0.0.0-20210909181612-21743d7dd02a // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/containerd/continuity v0.2.2 // indirect
	github.com/coredns/caddy v1.1.1 // indirect
	github.com/coredns/coredns v1.9.0 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.1 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dgraph-io/ristretto v0.1.0 // indirect
	github.com/dhui/dktest v0.3.9 // indirect
	github.com/dlclark/regexp2 v1.4.0 // indirect
	github.com/docker/cli v20.10.13+incompatible // indirect
	github.com/docker/distribution v2.8.0+incompatible // indirect
	github.com/docker/docker v20.10.13+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/dustin/go-humanize v1.0.0 // indirect
	github.com/facebookgo/grace v0.0.0-20180706040059-75cf19382434 // indirect
	github.com/flynn/go-shlex v0.0.0-20150515145356-3f9db97f8568 // indirect
	github.com/francoispqt/gojay v1.2.13 // indirect
	github.com/fsnotify/fsnotify v1.5.1 // indirect
	github.com/gdamore/encoding v1.0.0 // indirect
	github.com/gdamore/tcell v1.4.0 // indirect
	github.com/getsentry/raven-go v0.2.0 // indirect
	github.com/go-playground/locales v0.14.0 // indirect
	github.com/go-playground/universal-translator v0.18.0 // indirect
	github.com/go-task/slim-sprig v0.0.0-20210107165309-348f09dbbbc0 // indirect
	github.com/gobwas/glob v0.2.3 // indirect
	github.com/gobwas/httphead v0.1.0 // indirect
	github.com/gobwas/pool v0.2.1 // indirect
	github.com/gobwas/ws v1.1.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang-collections/collections v0.0.0-20130729185459-604e922904d3 // indirect
	github.com/golang/glog v0.0.0-20160126235308-23def4e6c14b // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/go-cmp v0.5.7 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/gorilla/mux v1.8.0 // indirect
	github.com/gorilla/websocket v1.5.0 // indirect
	github.com/grpc-ecosystem/grpc-opentracing v0.0.0-20180507213350-8e809c8a8645 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/terraform-json v0.13.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/juju/ansiterm v0.0.0-20210929141451-8b71cc96ebdc // indirect
	github.com/kballard/go-shellquote v0.0.0-20180428030007-95032a82bc51 // indirect
	github.com/klauspost/compress v1.15.0 // indirect
	github.com/leodido/go-urn v1.2.1 // indirect
	github.com/lucas-clemente/quic-go v0.25.1-0.20220307142123-ad1cb27c1b64 // indirect
	github.com/lucasb-eyer/go-colorful v1.2.0 // indirect
	github.com/lunixbochs/vtclean v1.0.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/marten-seemann/qtls-go1-16 v0.1.4 // indirect
	github.com/marten-seemann/qtls-go1-17 v0.1.0 // indirect
	github.com/marten-seemann/qtls-go1-18 v0.1.0-beta.1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-runewidth v0.0.13 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.2-0.20181231171920-c182affec369 // indirect
	github.com/mgutz/ansi v0.0.0-20170206155736-9520e82c474b // indirect
	github.com/miekg/dns v1.1.46 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/mitchellh/go-wordwrap v1.0.1 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/muesli/ansi v0.0.0-20211031195517-c9f0611b6c70 // indirect
	github.com/muesli/reflow v0.3.0 // indirect
	github.com/muesli/termenv v0.11.1-0.20220212125758-44cd13922739 // indirect
	github.com/niklasfasching/go-org v1.6.2 // indirect
	github.com/nxadm/tail v1.4.8 // indirect
	github.com/onsi/ginkgo v1.16.5 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opencontainers/runc v1.1.0 // indirect
	github.com/opentracing/opentracing-go v1.2.0 // indirect
	github.com/pelletier/go-toml/v2 v2.0.0-beta.6 // indirect
	github.com/philhofer/fwd v1.1.1 // indirect
	github.com/pion/dtls/v2 v2.1.3 // indirect
	github.com/pion/ice/v2 v2.2.2 // indirect
	github.com/pion/interceptor v0.1.10 // indirect
	github.com/pion/mdns v0.0.5 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.9 // indirect
	github.com/pion/rtp v1.7.9 // indirect
	github.com/pion/sctp v1.8.2 // indirect
	github.com/pion/sdp/v3 v3.0.4 // indirect
	github.com/pion/srtp/v2 v2.0.5 // indirect
	github.com/pion/stun v0.3.5 // indirect
	github.com/pion/turn/v2 v2.0.8 // indirect
	github.com/pion/udp v0.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/pquerna/cachecontrol v0.1.0 // indirect
	github.com/prometheus/client_golang v1.12.1 // indirect
	github.com/prometheus/client_model v0.2.0 // indirect
	github.com/prometheus/common v0.32.1 // indirect
	github.com/prometheus/procfs v0.7.3 // indirect
	github.com/rivo/tview v0.0.0-20200712113419-c65badfc3d92 // indirect
	github.com/rivo/uniseg v0.2.0 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/spf13/afero v1.8.1 // indirect
	github.com/spf13/cast v1.4.1 // indirect
	github.com/spf13/jwalterweatherman v1.1.0 // indirect
	github.com/tinylib/msgp v1.1.2 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20190905194746-02993c407bfb // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/zclconf/go-cty v1.10.0 // indirect
	github.com/zeebo/errs v1.2.2 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/mod v0.5.1 // indirect
	golang.org/x/net v0.0.0-20220225172249-27dd8689420f // indirect
	golang.org/x/term v0.0.0-20210927222741-03fcf44c2211 // indirect
	golang.org/x/text v0.3.7 // indirect
	golang.org/x/time v0.0.0-20211116232009-f0f3c7e86c11 // indirect
	golang.org/x/tools v0.1.9 // indirect
	google.golang.org/appengine v1.6.7 // indirect
	google.golang.org/genproto v0.0.0-20220310185008-1973136f34c6 // indirect
	google.golang.org/grpc v1.45.0 // indirect
	gopkg.in/DataDog/dd-trace-go.v1 v1.37.1
	gopkg.in/coreos/go-oidc.v2 v2.2.1 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.0.0 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
	gopkg.in/tomb.v1 v1.0.0-20141024135613-dd632973f1e7 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
	zombiezen.com/go/capnproto2 v2.18.2+incompatible // indirect
)
