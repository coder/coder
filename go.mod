module github.com/coder/coder

go 1.17

// Required until https://github.com/hashicorp/terraform-config-inspect/pull/74 is merged.
replace github.com/hashicorp/terraform-config-inspect => github.com/kylecarbs/terraform-config-inspect v0.0.0-20211215004401-bbc517866b88

// Required until https://github.com/pion/ice/pull/411 is merged.
replace github.com/pion/ice/v2 => github.com/kylecarbs/ice/v2 v2.1.8-0.20220113224934-e3297ead83b2

require (
	cdr.dev/slog v1.4.1
	github.com/go-chi/chi v1.5.4
	github.com/go-chi/render v1.0.1
	github.com/golang-migrate/migrate/v4 v4.15.1
	github.com/google/uuid v1.3.0
	github.com/hashicorp/go-version v1.3.0
	github.com/hashicorp/hc-install v0.3.1
	github.com/hashicorp/terraform-config-inspect v0.0.0-20211115214459-90acf1ca460f
	github.com/hashicorp/terraform-exec v0.15.0
	github.com/lib/pq v1.10.4
	github.com/ory/dockertest/v3 v3.8.1
	github.com/pion/datachannel v1.5.2
	github.com/pion/logging v0.2.2
	github.com/pion/transport v0.13.0
	github.com/pion/webrtc/v3 v3.1.13
	github.com/spf13/cobra v1.3.0
	github.com/stretchr/testify v1.7.0
	go.uber.org/atomic v1.7.0
	go.uber.org/goleak v1.1.12
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
	google.golang.org/protobuf v1.27.1
	storj.io/drpc v0.0.26
)

require (
	cloud.google.com/go/storage v1.14.0 // indirect
	github.com/Azure/go-ansiterm v0.0.0-20210617225240-d185dfc1b5a1 // indirect
	github.com/Microsoft/go-winio v0.5.1 // indirect
	github.com/Nvveen/Gotty v0.0.0-20120604004816-cd527374f1e5 // indirect
	github.com/agext/levenshtein v1.2.2 // indirect
	github.com/alecthomas/chroma v0.9.1 // indirect
	github.com/apparentlymart/go-textseg v1.0.0 // indirect
	github.com/apparentlymart/go-textseg/v13 v13.0.0 // indirect
	github.com/cenkalti/backoff/v4 v4.1.2 // indirect
	github.com/containerd/continuity v0.1.0 // indirect
	github.com/danwakefield/fnmatch v0.0.0-20160403171240-cbb64ac3d964 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dhui/dktest v0.3.8 // indirect
	github.com/dlclark/regexp2 v1.4.0 // indirect
	github.com/docker/cli v20.10.11+incompatible // indirect
	github.com/docker/docker v20.10.12+incompatible // indirect
	github.com/docker/go-connections v0.4.0 // indirect
	github.com/docker/go-units v0.4.0 // indirect
	github.com/fatih/color v1.13.0 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/google/go-cmp v0.5.6 // indirect
	github.com/google/shlex v0.0.0-20191202100458-e7afc7fbc510 // indirect
	github.com/hashicorp/errwrap v1.0.0 // indirect
	github.com/hashicorp/go-cleanhttp v0.5.2 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/hcl v1.0.0 // indirect
	github.com/hashicorp/hcl/v2 v2.0.0 // indirect
	github.com/hashicorp/terraform-json v0.13.0 // indirect
	github.com/imdario/mergo v0.3.12 // indirect
	github.com/inconshreveable/mousetrap v1.0.0 // indirect
	github.com/justinas/nosurf v1.1.1 // indirect
	github.com/mattn/go-colorable v0.1.12 // indirect
	github.com/mattn/go-isatty v0.0.14 // indirect
	github.com/mitchellh/go-wordwrap v1.0.0 // indirect
	github.com/mitchellh/mapstructure v1.4.3 // indirect
	github.com/moby/term v0.0.0-20210619224110-3f7ff695adc6 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.0.2 // indirect
	github.com/opencontainers/runc v1.0.2 // indirect
	github.com/pion/dtls/v2 v2.1.0 // indirect
	github.com/pion/ice/v2 v2.1.18 // indirect
	github.com/pion/interceptor v0.1.4 // indirect
	github.com/pion/mdns v0.0.5 // indirect
	github.com/pion/randutil v0.1.0 // indirect
	github.com/pion/rtcp v1.2.9 // indirect
	github.com/pion/rtp v1.7.4 // indirect
	github.com/pion/sctp v1.8.2 // indirect
	github.com/pion/sdp/v3 v3.0.4 // indirect
	github.com/pion/srtp/v2 v2.0.5 // indirect
	github.com/pion/stun v0.3.5 // indirect
	github.com/pion/turn/v2 v2.0.6 // indirect
	github.com/pion/udp v0.1.1 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/sirupsen/logrus v1.8.1 // indirect
	github.com/spf13/pflag v1.0.5 // indirect
	github.com/unrolled/secure v1.0.9 // indirect
	github.com/xeipuuv/gojsonpointer v0.0.0-20180127040702-4e3ac2762d5f // indirect
	github.com/xeipuuv/gojsonreference v0.0.0-20180127040603-bd5ef7bd5415 // indirect
	github.com/xeipuuv/gojsonschema v1.2.0 // indirect
	github.com/zclconf/go-cty v1.9.1 // indirect
	github.com/zeebo/errs v1.2.2 // indirect
	go.opencensus.io v0.23.0 // indirect
	golang.org/x/crypto v0.0.0-20211215153901-e495a2d5b3d3 // indirect
	golang.org/x/net v0.0.0-20211216030914-fe4d6282115f // indirect
	golang.org/x/sys v0.0.0-20211210111614-af8b64212486 // indirect
	golang.org/x/term v0.0.0-20210615171337-6886f2dfbf5b // indirect
	golang.org/x/text v0.3.7 // indirect
	google.golang.org/api v0.63.0 // indirect
	google.golang.org/grpc v1.43.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.0-20210107192922-496545a6307b // indirect
)
