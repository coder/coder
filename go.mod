module github.com/coder/coder/v2

go 1.26.4

// Required until a v3 of chroma is created to lazily initialize all XML files.
// None of our dependencies seem to use the registries anyways, so this
// should be fine...
// See: https://github.com/kylecarbs/chroma/commit/9e036e0631f38ef60de5ee8eec7a42e9cb7da423
replace github.com/alecthomas/chroma/v2 => github.com/kylecarbs/chroma/v2 v2.0.0-20240401211003-9e036e0631f3

// Required until https://github.com/hashicorp/terraform-config-inspect/pull/74 is merged.
replace github.com/hashicorp/terraform-config-inspect => github.com/coder/terraform-config-inspect v0.0.0-20250107175719-6d06d90c630e

// Required until https://github.com/chzyer/readline/pull/198 is merged.
replace github.com/chzyer/readline => github.com/kylecarbs/readline v0.0.0-20220211054233-0d62993714c8

// Required until https://github.com/briandowns/spinner/pull/136 is merged.
replace github.com/briandowns/spinner => github.com/kylecarbs/spinner v1.18.2-0.20220329160715-20702b5af89e

// opencensus-go leaks a goroutine by default.
replace go.opencensus.io => github.com/kylecarbs/opencensus-go v0.23.1-0.20220307014935-4d0325a68f8b

// See https://github.com/kylecarbs/tview/commit/8464256e10a1e85074c7ef9c80346baa60e9ede6
replace github.com/rivo/tview => github.com/kylecarbs/tview v0.0.0-20220309202238-8464256e10a1

// glog has a single goroutine leak on start that we removed in a fork: https://github.com/coder/glog/pull/1.
replace github.com/golang/glog => github.com/coder/glog v1.0.1-0.20220322161911-7365fe7f2cd1

// kcp-go starts a goroutine in an init function that we can't stop. It was
// fixed in our fork:
// https://github.com/coder/kcp-go/commit/83c0904cec69dcf21ec10c54ea666bda18ada831
replace github.com/fatedier/kcp-go => github.com/coder/kcp-go v2.0.4-0.20220409183554-83c0904cec69+incompatible

// https://github.com/tcnksm/go-httpstat/pull/29
replace github.com/tcnksm/go-httpstat => github.com/coder/go-httpstat v0.0.0-20230801153223-321c88088322

// There are a few minor changes we make to Tailscale that we're slowly upstreaming. Compare here:
// https://github.com/tailscale/tailscale/compare/main...coder:tailscale:main
replace tailscale.com => github.com/coder/tailscale v1.1.1-0.20260529105257-b7c5fc6e6399

// This is replaced to include
// 1. a fix for a data race: c.f. https://github.com/tailscale/wireguard-go/pull/25
// 2. update to the latest gVisor
replace github.com/tailscale/wireguard-go => github.com/coder/wireguard-go v0.0.0-20260113101225-9b7a56210e49

// We use a fork to fix an integer overflow issue that causes occasional crashes in workspace agents.
// See https://github.com/coder/coder/issues/20885
replace gvisor.dev/gvisor => github.com/coder/gvisor v0.0.0-20260313164934-7a658db7b714
