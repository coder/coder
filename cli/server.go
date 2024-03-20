//go:build !slim

package cli

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/pprof"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/coreos/go-systemd/daemon"
	embeddedpostgres "github.com/fergusstrange/embedded-postgres"
	"github.com/google/go-github/v43/github"
	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/mod/semver"
	"golang.org/x/oauth2"
	xgithub "golang.org/x/oauth2/github"
	"golang.org/x/sync/errgroup"
	"golang.org/x/xerrors"
	"google.golang.org/api/idtoken"
	"google.golang.org/api/option"
	"gopkg.in/yaml.v3"
	"tailscale.com/tailcfg"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/buildinfo"
	"github.com/coder/coder/v2/cli/clilog"
	"github.com/coder/coder/v2/cli/cliui"
	"github.com/coder/coder/v2/cli/cliutil"
	"github.com/coder/coder/v2/cli/config"
	"github.com/coder/coder/v2/coderd"
	"github.com/coder/coder/v2/coderd/autobuild"
	"github.com/coder/coder/v2/coderd/batchstats"
	"github.com/coder/coder/v2/coderd/database"
	"github.com/coder/coder/v2/coderd/database/dbmem"
	"github.com/coder/coder/v2/coderd/database/dbmetrics"
	"github.com/coder/coder/v2/coderd/database/dbpurge"
	"github.com/coder/coder/v2/coderd/database/migrations"
	"github.com/coder/coder/v2/coderd/database/pubsub"
	"github.com/coder/coder/v2/coderd/devtunnel"
	"github.com/coder/coder/v2/coderd/externalauth"
	"github.com/coder/coder/v2/coderd/gitsshkey"
	"github.com/coder/coder/v2/coderd/httpmw"
	"github.com/coder/coder/v2/coderd/oauthpki"
	"github.com/coder/coder/v2/coderd/prometheusmetrics"
	"github.com/coder/coder/v2/coderd/prometheusmetrics/insights"
	"github.com/coder/coder/v2/coderd/promoauth"
	"github.com/coder/coder/v2/coderd/schedule"
	"github.com/coder/coder/v2/coderd/telemetry"
	"github.com/coder/coder/v2/coderd/tracing"
	"github.com/coder/coder/v2/coderd/unhanger"
	"github.com/coder/coder/v2/coderd/updatecheck"
	"github.com/coder/coder/v2/coderd/util/slice"
	stringutil "github.com/coder/coder/v2/coderd/util/strings"
	"github.com/coder/coder/v2/coderd/workspaceapps"
	"github.com/coder/coder/v2/coderd/workspaceapps/appurl"
	"github.com/coder/coder/v2/coderd/workspaceusage"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/codersdk/drpc"
	"github.com/coder/coder/v2/cryptorand"
	"github.com/coder/coder/v2/provisioner/echo"
	"github.com/coder/coder/v2/provisioner/terraform"
	"github.com/coder/coder/v2/provisionerd"
	"github.com/coder/coder/v2/provisionerd/proto"
	"github.com/coder/coder/v2/provisionersdk"
	sdkproto "github.com/coder/coder/v2/provisionersdk/proto"
	"github.com/coder/coder/v2/tailnet"
	"github.com/coder/pretty"
	"github.com/coder/retry"
	"github.com/coder/serpent"
	"github.com/coder/wgtunnel/tunnelsdk"
)

func createOIDCConfig(ctx context.Context, vals *codersdk.DeploymentValues) (*coderd.OIDCConfig, error) {
	if vals.OIDC.ClientID == "" {
		return nil, xerrors.Errorf("OIDC client ID must be set!")
	}
	if vals.OIDC.IssuerURL == "" {
		return nil, xerrors.Errorf("OIDC issuer URL must be set!")
	}

	oidcProvider, err := oidc.NewProvider(
		ctx, vals.OIDC.IssuerURL.String(),
	)
	if err != nil {
		return nil, xerrors.Errorf("configure oidc provider: %w", err)
	}
	redirectURL, err := vals.AccessURL.Value().Parse("/api/v2/users/oidc/callback")
	if err != nil {
		return nil, xerrors.Errorf("parse oidc oauth callback url: %w", err)
	}
	// If the scopes contain 'groups', we enable group support.
	// Do not override any custom value set by the user.
	if slice.Contains(vals.OIDC.Scopes, "groups") && vals.OIDC.GroupField == "" {
		vals.OIDC.GroupField = "groups"
	}
	oauthCfg := &oauth2.Config{
		ClientID:     vals.OIDC.ClientID.String(),
		ClientSecret: vals.OIDC.ClientSecret.String(),
		RedirectURL:  redirectURL.String(),
		Endpoint:     oidcProvider.Endpoint(),
		Scopes:       vals.OIDC.Scopes,
	}

	var useCfg promoauth.OAuth2Config = oauthCfg
	if vals.OIDC.ClientKeyFile != "" {
		// PKI authentication is done in the params. If a
		// counter example is found, we can add a config option to
		// change this.
		oauthCfg.Endpoint.AuthStyle = oauth2.AuthStyleInParams
		if vals.OIDC.ClientSecret != "" {
			return nil, xerrors.Errorf("cannot specify both oidc client secret and oidc client key file")
		}

		pkiCfg, err := configureOIDCPKI(oauthCfg, vals.OIDC.ClientKeyFile.Value(), vals.OIDC.ClientCertFile.Value())
		if err != nil {
			return nil, xerrors.Errorf("configure oauth pki authentication: %w", err)
		}
		useCfg = pkiCfg
	}
	if len(vals.OIDC.GroupAllowList) > 0 && vals.OIDC.GroupField == "" {
		return nil, xerrors.Errorf("'oidc-group-field' must be set if 'oidc-allowed-groups' is set. Either unset 'oidc-allowed-groups' or set 'oidc-group-field'")
	}

	groupAllowList := make(map[string]bool)
	for _, group := range vals.OIDC.GroupAllowList.Value() {
		groupAllowList[group] = true
	}

	return &coderd.OIDCConfig{
		OAuth2Config: useCfg,
		Provider:     oidcProvider,
		Verifier: oidcProvider.Verifier(&oidc.Config{
			ClientID: vals.OIDC.ClientID.String(),
		}),
		EmailDomain:         vals.OIDC.EmailDomain,
		AllowSignups:        vals.OIDC.AllowSignups.Value(),
		UsernameField:       vals.OIDC.UsernameField.String(),
		EmailField:          vals.OIDC.EmailField.String(),
		AuthURLParams:       vals.OIDC.AuthURLParams.Value,
		IgnoreUserInfo:      vals.OIDC.IgnoreUserInfo.Value(),
		GroupField:          vals.OIDC.GroupField.String(),
		GroupFilter:         vals.OIDC.GroupRegexFilter.Value(),
		GroupAllowList:      groupAllowList,
		CreateMissingGroups: vals.OIDC.GroupAutoCreate.Value(),
		GroupMapping:        vals.OIDC.GroupMapping.Value,
		UserRoleField:       vals.OIDC.UserRoleField.String(),
		UserRoleMapping:     vals.OIDC.UserRoleMapping.Value,
		UserRolesDefault:    vals.OIDC.UserRolesDefault.GetSlice(),
		SignInText:          vals.OIDC.SignInText.String(),
		SignupsDisabledText: vals.OIDC.SignupsDisabledText.String(),
		IconURL:             vals.OIDC.IconURL.String(),
		IgnoreEmailVerified: vals.OIDC.IgnoreEmailVerified.Value(),
	}, nil
}

func afterCtx(ctx context.Context, fn func()) {
	go func() {
		<-ctx.Done()
		fn()
	}()
}

func enablePrometheus(
	ctx context.Context,
	logger slog.Logger,
	vals *codersdk.DeploymentValues,
	options *coderd.Options,
) (closeFn func(), err error) {
	options.PrometheusRegistry.MustRegister(collectors.NewGoCollector())
	options.PrometheusRegistry.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))

	closeUsersFunc, err := prometheusmetrics.ActiveUsers(ctx, options.PrometheusRegistry, options.Database, 0)
	if err != nil {
		return nil, xerrors.Errorf("register active users prometheus metric: %w", err)
	}
	afterCtx(ctx, closeUsersFunc)

	closeWorkspacesFunc, err := prometheusmetrics.Workspaces(ctx, options.PrometheusRegistry, options.Database, 0)
	if err != nil {
		return nil, xerrors.Errorf("register workspaces prometheus metric: %w", err)
	}
	afterCtx(ctx, closeWorkspacesFunc)

	insightsMetricsCollector, err := insights.NewMetricsCollector(options.Database, options.Logger, 0, 0)
	if err != nil {
		return nil, xerrors.Errorf("unable to initialize insights metrics collector: %w", err)
	}
	err = options.PrometheusRegistry.Register(insightsMetricsCollector)
	if err != nil {
		return nil, xerrors.Errorf("unable to register insights metrics collector: %w", err)
	}

	closeInsightsMetricsCollector, err := insightsMetricsCollector.Run(ctx)
	if err != nil {
		return nil, xerrors.Errorf("unable to run insights metrics collector: %w", err)
	}
	afterCtx(ctx, closeInsightsMetricsCollector)

	if vals.Prometheus.CollectAgentStats {
		closeAgentStatsFunc, err := prometheusmetrics.AgentStats(ctx, logger, options.PrometheusRegistry, options.Database, time.Now(), 0, options.DeploymentValues.Prometheus.AggregateAgentStatsBy.Value())
		if err != nil {
			return nil, xerrors.Errorf("register agent stats prometheus metric: %w", err)
		}
		afterCtx(ctx, closeAgentStatsFunc)

		metricsAggregator, err := prometheusmetrics.NewMetricsAggregator(logger, options.PrometheusRegistry, 0, options.DeploymentValues.Prometheus.AggregateAgentStatsBy.Value())
		if err != nil {
			return nil, xerrors.Errorf("can't initialize metrics aggregator: %w", err)
		}

		cancelMetricsAggregator := metricsAggregator.Run(ctx)
		afterCtx(ctx, cancelMetricsAggregator)

		options.UpdateAgentMetrics = metricsAggregator.Update
		err = options.PrometheusRegistry.Register(metricsAggregator)
		if err != nil {
			return nil, xerrors.Errorf("can't register metrics aggregator as collector: %w", err)
		}
	}

	//nolint:revive
	return ServeHandler(
		ctx, logger, promhttp.InstrumentMetricHandler(
			options.PrometheusRegistry, promhttp.HandlerFor(options.PrometheusRegistry, promhttp.HandlerOpts{}),
		), vals.Prometheus.Address.String(), "prometheus",
	), nil
}

//nolint:gocognit // TODO(dannyk): reduce complexity of this function
func (r *RootCmd) Server(newAPI func(context.Context, *coderd.Options) (*coderd.API, io.Closer, error)) *serpent.Command {
	if newAPI == nil {
		newAPI = func(_ context.Context, o *coderd.Options) (*coderd.API, io.Closer, error) {
			api := coderd.New(o)
			return api, api, nil
		}
	}

	var (
		vals = new(codersdk.DeploymentValues)
		opts = vals.Options()
	)
	serverCmd := &serpent.Command{
		Use:     "server",
		Short:   "Start a Coder server",
		Options: opts,
		Middleware: serpent.Chain(
			WriteConfigMW(vals),
			PrintDeprecatedOptions(),
			serpent.RequireNArgs(0),
		),
		Handler: func(inv *serpent.Invocation) error {
			// Main command context for managing cancellation of running
			// services.
			ctx, cancel := context.WithCancel(inv.Context())
			defer cancel()

			if vals.Config != "" {
				cliui.Warnf(inv.Stderr, "YAML support is experimental and offers no compatibility guarantees.")
			}

			go DumpHandler(ctx, "coderd")

			// Validate bind addresses.
			if vals.Address.String() != "" {
				if vals.TLS.Enable {
					vals.HTTPAddress = ""
					vals.TLS.Address = vals.Address
				} else {
					_ = vals.HTTPAddress.Set(vals.Address.String())
					vals.TLS.Address.Host = ""
					vals.TLS.Address.Port = ""
				}
			}
			if vals.TLS.Enable && vals.TLS.Address.String() == "" {
				return xerrors.Errorf("TLS address must be set if TLS is enabled")
			}
			if !vals.TLS.Enable && vals.HTTPAddress.String() == "" {
				return xerrors.Errorf("TLS is disabled. Enable with --tls-enable or specify a HTTP address")
			}

			if vals.AccessURL.String() != "" &&
				!(vals.AccessURL.Scheme == "http" || vals.AccessURL.Scheme == "https") {
				return xerrors.Errorf("access-url must include a scheme (e.g. 'http://' or 'https://)")
			}

			// Disable rate limits if the `--dangerous-disable-rate-limits` flag
			// was specified.
			loginRateLimit := 60
			filesRateLimit := 12
			if vals.RateLimit.DisableAll {
				vals.RateLimit.API = -1
				loginRateLimit = -1
				filesRateLimit = -1
			}

			PrintLogo(inv, "Coder")
			logger, logCloser, err := clilog.New(clilog.FromDeploymentValues(vals)).Build(inv)
			if err != nil {
				return xerrors.Errorf("make logger: %w", err)
			}
			defer logCloser()

			// This line is helpful in tests.
			logger.Debug(ctx, "started debug logging")
			logger.Sync()

			// Register signals early on so that graceful shutdown can't
			// be interrupted by additional signals. Note that we avoid
			// shadowing cancel() (from above) here because stopCancel()
			// restores default behavior for the signals. This protects
			// the shutdown sequence from abruptly terminating things
			// like: database migrations, provisioner work, workspace
			// cleanup in dev-mode, etc.
			//
			// To get out of a graceful shutdown, the user can send
			// SIGQUIT with ctrl+\ or SIGKILL with `kill -9`.
			stopCtx, stopCancel := signalNotifyContext(ctx, inv, StopSignalsNoInterrupt...)
			defer stopCancel()
			interruptCtx, interruptCancel := signalNotifyContext(ctx, inv, InterruptSignals...)
			defer interruptCancel()

			cacheDir := vals.CacheDir.String()
			err = os.MkdirAll(cacheDir, 0o700)
			if err != nil {
				return xerrors.Errorf("create cache directory: %w", err)
			}

			// Clean up idle connections at the end, e.g.
			// embedded-postgres can leave an idle connection
			// which is caught by goleaks.
			defer http.DefaultClient.CloseIdleConnections()

			tracerProvider, sqlDriver, closeTracing := ConfigureTraceProvider(ctx, logger, vals)
			defer func() {
				logger.Debug(ctx, "closing tracing")
				traceCloseErr := shutdownWithTimeout(closeTracing, 5*time.Second)
				logger.Debug(ctx, "tracing closed", slog.Error(traceCloseErr))
			}()

			httpServers, err := ConfigureHTTPServers(logger, inv, vals)
			if err != nil {
				return xerrors.Errorf("configure http(s): %w", err)
			}
			defer httpServers.Close()

			config := r.createConfig()

			builtinPostgres := false
			// Only use built-in if PostgreSQL URL isn't specified!
			if !vals.InMemoryDatabase && vals.PostgresURL == "" {
				var closeFunc func() error
				cliui.Infof(inv.Stdout, "Using built-in PostgreSQL (%s)", config.PostgresPath())
				pgURL, closeFunc, err := startBuiltinPostgres(ctx, config, logger)
				if err != nil {
					return err
				}

				err = vals.PostgresURL.Set(pgURL)
				if err != nil {
					return err
				}
				builtinPostgres = true
				defer func() {
					cliui.Infof(inv.Stdout, "Stopping built-in PostgreSQL...")
					// Gracefully shut PostgreSQL down!
					if err := closeFunc(); err != nil {
						cliui.Errorf(inv.Stderr, "Failed to stop built-in PostgreSQL: %v", err)
					} else {
						cliui.Infof(inv.Stdout, "Stopped built-in PostgreSQL")
					}
				}()
			}

			// Prefer HTTP because it's less prone to TLS errors over localhost.
			localURL := httpServers.TLSUrl
			if httpServers.HTTPUrl != nil {
				localURL = httpServers.HTTPUrl
			}

			ctx, httpClient, err := ConfigureHTTPClient(
				ctx,
				vals.TLS.ClientCertFile.String(),
				vals.TLS.ClientKeyFile.String(),
				vals.TLS.ClientCAFile.String(),
			)
			if err != nil {
				return xerrors.Errorf("configure http client: %w", err)
			}

			// If the access URL is empty, we attempt to run a reverse-proxy
			// tunnel to make the initial setup really simple.
			var (
				tunnel     *tunnelsdk.Tunnel
				tunnelDone <-chan struct{} = make(chan struct{}, 1)
			)
			if vals.AccessURL.String() == "" {
				cliui.Infof(inv.Stderr, "Opening tunnel so workspaces can connect to your deployment. For production scenarios, specify an external access URL")
				tunnel, err = devtunnel.New(ctx, logger.Named("net.devtunnel"), vals.WgtunnelHost.String())
				if err != nil {
					return xerrors.Errorf("create tunnel: %w", err)
				}
				defer tunnel.Close()
				tunnelDone = tunnel.Wait()
				vals.AccessURL = serpent.URL(*tunnel.URL)

				if vals.WildcardAccessURL.String() == "" {
					// Suffixed wildcard access URL.
					wu := fmt.Sprintf("*--%s", tunnel.URL.Hostname())
					err = vals.WildcardAccessURL.Set(wu)
					if err != nil {
						return xerrors.Errorf("set wildcard access url %q: %w", wu, err)
					}
				}
			}

			_, accessURLPortRaw, _ := net.SplitHostPort(vals.AccessURL.Host)
			if accessURLPortRaw == "" {
				accessURLPortRaw = "80"
				if vals.AccessURL.Scheme == "https" {
					accessURLPortRaw = "443"
				}
			}

			accessURLPort, err := strconv.Atoi(accessURLPortRaw)
			if err != nil {
				return xerrors.Errorf("parse access URL port: %w", err)
			}

			// Warn the user if the access URL is loopback or unresolvable.
			isLocal, err := IsLocalURL(ctx, vals.AccessURL.Value())
			if isLocal || err != nil {
				reason := "could not be resolved"
				if isLocal {
					reason = "isn't externally reachable"
				}
				cliui.Warnf(
					inv.Stderr,
					"The access URL %s %s, this may cause unexpected problems when creating workspaces. Generate a unique *.try.coder.app URL by not specifying an access URL.\n",
					pretty.Sprint(cliui.DefaultStyles.Field, vals.AccessURL.String()), reason,
				)
			}

			// A newline is added before for visibility in terminal output.
			cliui.Infof(inv.Stdout, "\nView the Web UI: %s", vals.AccessURL.String())

			// Used for zero-trust instance identity with Google Cloud.
			googleTokenValidator, err := idtoken.NewValidator(ctx, option.WithoutAuthentication())
			if err != nil {
				return err
			}

			sshKeygenAlgorithm, err := gitsshkey.ParseAlgorithm(vals.SSHKeygenAlgorithm.String())
			if err != nil {
				return xerrors.Errorf("parse ssh keygen algorithm %s: %w", vals.SSHKeygenAlgorithm, err)
			}

			defaultRegion := &tailcfg.DERPRegion{
				EmbeddedRelay: true,
				RegionID:      int(vals.DERP.Server.RegionID.Value()),
				RegionCode:    vals.DERP.Server.RegionCode.String(),
				RegionName:    vals.DERP.Server.RegionName.String(),
				Nodes: []*tailcfg.DERPNode{{
					Name:      fmt.Sprintf("%db", vals.DERP.Server.RegionID),
					RegionID:  int(vals.DERP.Server.RegionID.Value()),
					HostName:  vals.AccessURL.Value().Hostname(),
					DERPPort:  accessURLPort,
					STUNPort:  -1,
					ForceHTTP: vals.AccessURL.Scheme == "http",
				}},
			}
			if !vals.DERP.Server.Enable {
				defaultRegion = nil
			}

			derpMap, err := tailnet.NewDERPMap(
				ctx, defaultRegion, vals.DERP.Server.STUNAddresses,
				vals.DERP.Config.URL.String(), vals.DERP.Config.Path.String(),
				vals.DERP.Config.BlockDirect.Value(),
			)
			if err != nil {
				return xerrors.Errorf("create derp map: %w", err)
			}

			appHostname := vals.WildcardAccessURL.String()
			var appHostnameRegex *regexp.Regexp
			if appHostname != "" {
				appHostnameRegex, err = appurl.CompileHostnamePattern(appHostname)
				if err != nil {
					return xerrors.Errorf("parse wildcard access URL %q: %w", appHostname, err)
				}
			}

			extAuthEnv, err := ReadExternalAuthProvidersFromEnv(os.Environ())
			if err != nil {
				return xerrors.Errorf("read external auth providers from env: %w", err)
			}

			promRegistry := prometheus.NewRegistry()
			oauthInstrument := promoauth.NewFactory(promRegistry)
			vals.ExternalAuthConfigs.Value = append(vals.ExternalAuthConfigs.Value, extAuthEnv...)
			externalAuthConfigs, err := externalauth.ConvertConfig(
				oauthInstrument,
				vals.ExternalAuthConfigs.Value,
				vals.AccessURL.Value(),
			)
			if err != nil {
				return xerrors.Errorf("convert external auth config: %w", err)
			}
			for _, c := range externalAuthConfigs {
				logger.Debug(
					ctx, "loaded external auth config",
					slog.F("id", c.ID),
				)
			}

			realIPConfig, err := httpmw.ParseRealIPConfig(vals.ProxyTrustedHeaders, vals.ProxyTrustedOrigins)
			if err != nil {
				return xerrors.Errorf("parse real ip config: %w", err)
			}

			configSSHOptions, err := vals.SSHConfig.ParseOptions()
			if err != nil {
				return xerrors.Errorf("parse ssh config options %q: %w", vals.SSHConfig.SSHConfigOptions.String(), err)
			}

			options := &coderd.Options{
				AccessURL:                   vals.AccessURL.Value(),
				AppHostname:                 appHostname,
				AppHostnameRegex:            appHostnameRegex,
				Logger:                      logger.Named("coderd"),
				Database:                    dbmem.New(),
				BaseDERPMap:                 derpMap,
				Pubsub:                      pubsub.NewInMemory(),
				CacheDir:                    cacheDir,
				GoogleTokenValidator:        googleTokenValidator,
				ExternalAuthConfigs:         externalAuthConfigs,
				RealIPConfig:                realIPConfig,
				SecureAuthCookie:            vals.SecureAuthCookie.Value(),
				SSHKeygenAlgorithm:          sshKeygenAlgorithm,
				TracerProvider:              tracerProvider,
				Telemetry:                   telemetry.NewNoop(),
				MetricsCacheRefreshInterval: vals.MetricsCacheRefreshInterval.Value(),
				AgentStatsRefreshInterval:   vals.AgentStatRefreshInterval.Value(),
				DeploymentValues:            vals,
				// Do not pass secret values to DeploymentOptions. All values should be read from
				// the DeploymentValues instead, this just serves to indicate the source of each
				// option. This is just defensive to prevent accidentally leaking.
				DeploymentOptions:           codersdk.DeploymentOptionsWithoutSecrets(opts),
				PrometheusRegistry:          promRegistry,
				APIRateLimit:                int(vals.RateLimit.API.Value()),
				LoginRateLimit:              loginRateLimit,
				FilesRateLimit:              filesRateLimit,
				HTTPClient:                  httpClient,
				TemplateScheduleStore:       &atomic.Pointer[schedule.TemplateScheduleStore]{},
				UserQuietHoursScheduleStore: &atomic.Pointer[schedule.UserQuietHoursScheduleStore]{},
				SSHConfig: codersdk.SSHConfigResponse{
					HostnamePrefix:   vals.SSHConfig.DeploymentName.String(),
					SSHConfigOptions: configSSHOptions,
				},
				AllowWorkspaceRenames: vals.AllowWorkspaceRenames.Value(),
			}
			if httpServers.TLSConfig != nil {
				options.TLSCertificates = httpServers.TLSConfig.Certificates
			}

			if vals.StrictTransportSecurity > 0 {
				options.StrictTransportSecurityCfg, err = httpmw.HSTSConfigOptions(
					int(vals.StrictTransportSecurity.Value()), vals.StrictTransportSecurityOptions,
				)
				if err != nil {
					return xerrors.Errorf("coderd: setting hsts header failed (options: %v): %w", vals.StrictTransportSecurityOptions, err)
				}
			}

			if vals.UpdateCheck {
				options.UpdateCheckOptions = &updatecheck.Options{
					// Avoid spamming GitHub API checking for updates.
					Interval: 24 * time.Hour,
					// Inform server admins of new versions.
					Notify: func(r updatecheck.Result) {
						if semver.Compare(r.Version, buildinfo.Version()) > 0 {
							options.Logger.Info(
								context.Background(),
								"new version of coder available",
								slog.F("new_version", r.Version),
								slog.F("url", r.URL),
								slog.F("upgrade_instructions", "https://coder.com/docs/coder-oss/latest/admin/upgrade"),
							)
						}
					},
				}
			}

			if vals.OAuth2.Github.ClientSecret != "" {
				options.GithubOAuth2Config, err = configureGithubOAuth2(
					oauthInstrument,
					vals.AccessURL.Value(),
					vals.OAuth2.Github.ClientID.String(),
					vals.OAuth2.Github.ClientSecret.String(),
					vals.OAuth2.Github.AllowSignups.Value(),
					vals.OAuth2.Github.AllowEveryone.Value(),
					vals.OAuth2.Github.AllowedOrgs,
					vals.OAuth2.Github.AllowedTeams,
					vals.OAuth2.Github.EnterpriseBaseURL.String(),
				)
				if err != nil {
					return xerrors.Errorf("configure github oauth2: %w", err)
				}
			}

			if vals.OIDC.ClientKeyFile != "" || vals.OIDC.ClientSecret != "" {
				if vals.OIDC.IgnoreEmailVerified {
					logger.Warn(ctx, "coder will not check email_verified for OIDC logins")
				}

				// This OIDC config is **not** being instrumented with the
				// oauth2 instrument wrapper. If we implement the missing
				// oidc methods, then we can instrument it.
				// Missing:
				//	- Userinfo
				//	- Verify
				oc, err := createOIDCConfig(ctx, vals)
				if err != nil {
					return xerrors.Errorf("create oidc config: %w", err)
				}
				options.OIDCConfig = oc
			}

			// We'll read from this channel in the select below that tracks shutdown.  If it remains
			// nil, that case of the select will just never fire, but it's important not to have a
			// "bare" read on this channel.
			var pubsubWatchdogTimeout <-chan struct{}
			if vals.InMemoryDatabase {
				// This is only used for testing.
				options.Database = dbmem.New()
				options.Pubsub = pubsub.NewInMemory()
			} else {
				dbURL, err := escapePostgresURLUserInfo(vals.PostgresURL.String())
				if err != nil {
					return xerrors.Errorf("escaping postgres URL: %w", err)
				}

				sqlDB, err := ConnectToPostgres(ctx, logger, sqlDriver, dbURL)
				if err != nil {
					return xerrors.Errorf("connect to postgres: %w", err)
				}
				defer func() {
					_ = sqlDB.Close()
				}()

				options.Database = database.New(sqlDB)
				ps, err := pubsub.New(ctx, logger.Named("pubsub"), sqlDB, dbURL)
				if err != nil {
					return xerrors.Errorf("create pubsub: %w", err)
				}
				options.Pubsub = ps
				if options.DeploymentValues.Prometheus.Enable {
					options.PrometheusRegistry.MustRegister(ps)
				}
				defer options.Pubsub.Close()
				psWatchdog := pubsub.NewWatchdog(ctx, logger.Named("pswatch"), ps)
				pubsubWatchdogTimeout = psWatchdog.Timeout()
				defer psWatchdog.Close()
			}

			if options.DeploymentValues.Prometheus.Enable && options.DeploymentValues.Prometheus.CollectDBMetrics {
				options.Database = dbmetrics.New(options.Database, options.PrometheusRegistry)
			}

			var deploymentID string
			err = options.Database.InTx(func(tx database.Store) error {
				// This will block until the lock is acquired, and will be
				// automatically released when the transaction ends.
				err := tx.AcquireLock(ctx, database.LockIDDeploymentSetup)
				if err != nil {
					return xerrors.Errorf("acquire lock: %w", err)
				}

				deploymentID, err = tx.GetDeploymentID(ctx)
				if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
					return xerrors.Errorf("get deployment id: %w", err)
				}
				if deploymentID == "" {
					deploymentID = uuid.NewString()
					err = tx.InsertDeploymentID(ctx, deploymentID)
					if err != nil {
						return xerrors.Errorf("set deployment id: %w", err)
					}
				}

				// Read the app signing key from the DB. We store it hex encoded
				// since the config table uses strings for the value and we
				// don't want to deal with automatic encoding issues.
				appSecurityKeyStr, err := tx.GetAppSecurityKey(ctx)
				if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
					return xerrors.Errorf("get app signing key: %w", err)
				}
				// If the string in the DB is an invalid hex string or the
				// length is not equal to the current key length, generate a new
				// one.
				//
				// If the key is regenerated, old signed tokens and encrypted
				// strings will become invalid. New signed app tokens will be
				// generated automatically on failure. Any workspace app token
				// smuggling operations in progress may fail, although with a
				// helpful error.
				if decoded, err := hex.DecodeString(appSecurityKeyStr); err != nil || len(decoded) != len(workspaceapps.SecurityKey{}) {
					b := make([]byte, len(workspaceapps.SecurityKey{}))
					_, err := rand.Read(b)
					if err != nil {
						return xerrors.Errorf("generate fresh app signing key: %w", err)
					}

					appSecurityKeyStr = hex.EncodeToString(b)
					err = tx.UpsertAppSecurityKey(ctx, appSecurityKeyStr)
					if err != nil {
						return xerrors.Errorf("insert freshly generated app signing key to database: %w", err)
					}
				}

				appSecurityKey, err := workspaceapps.KeyFromString(appSecurityKeyStr)
				if err != nil {
					return xerrors.Errorf("decode app signing key from database: %w", err)
				}

				options.AppSecurityKey = appSecurityKey

				// Read the oauth signing key from the database. Like the app security, generate a new one
				// if it is invalid for any reason.
				oauthSigningKeyStr, err := tx.GetOAuthSigningKey(ctx)
				if err != nil && !xerrors.Is(err, sql.ErrNoRows) {
					return xerrors.Errorf("get app oauth signing key: %w", err)
				}
				if decoded, err := hex.DecodeString(oauthSigningKeyStr); err != nil || len(decoded) != len(options.OAuthSigningKey) {
					b := make([]byte, len(options.OAuthSigningKey))
					_, err := rand.Read(b)
					if err != nil {
						return xerrors.Errorf("generate fresh oauth signing key: %w", err)
					}

					oauthSigningKeyStr = hex.EncodeToString(b)
					err = tx.UpsertOAuthSigningKey(ctx, oauthSigningKeyStr)
					if err != nil {
						return xerrors.Errorf("insert freshly generated oauth signing key to database: %w", err)
					}
				}

				keyBytes, err := hex.DecodeString(oauthSigningKeyStr)
				if err != nil {
					return xerrors.Errorf("decode oauth signing key from database: %w", err)
				}
				if len(keyBytes) != len(options.OAuthSigningKey) {
					return xerrors.Errorf("oauth signing key in database is not the correct length, expect %d got %d", len(options.OAuthSigningKey), len(keyBytes))
				}
				copy(options.OAuthSigningKey[:], keyBytes)
				if options.OAuthSigningKey == [32]byte{} {
					return xerrors.Errorf("oauth signing key in database is empty")
				}

				return nil
			}, nil)
			if err != nil {
				return err
			}

			if vals.Telemetry.Enable {
				gitAuth := make([]telemetry.GitAuth, 0)
				// TODO:
				var gitAuthConfigs []codersdk.ExternalAuthConfig
				for _, cfg := range gitAuthConfigs {
					gitAuth = append(gitAuth, telemetry.GitAuth{
						Type: cfg.Type,
					})
				}

				options.Telemetry, err = telemetry.New(telemetry.Options{
					BuiltinPostgres:    builtinPostgres,
					DeploymentID:       deploymentID,
					Database:           options.Database,
					Logger:             logger.Named("telemetry"),
					URL:                vals.Telemetry.URL.Value(),
					Wildcard:           vals.WildcardAccessURL.String() != "",
					DERPServerRelayURL: vals.DERP.Server.RelayURL.String(),
					GitAuth:            gitAuth,
					GitHubOAuth:        vals.OAuth2.Github.ClientID != "",
					OIDCAuth:           vals.OIDC.ClientID != "",
					OIDCIssuerURL:      vals.OIDC.IssuerURL.String(),
					Prometheus:         vals.Prometheus.Enable.Value(),
					STUN:               len(vals.DERP.Server.STUNAddresses) != 0,
					Tunnel:             tunnel != nil,
					Experiments:        vals.Experiments.Value(),
					ParseLicenseJWT: func(lic *telemetry.License) error {
						// This will be nil when running in AGPL-only mode.
						if options.ParseLicenseClaims == nil {
							return nil
						}

						email, trial, err := options.ParseLicenseClaims(lic.JWT)
						if err != nil {
							return err
						}
						if email != "" {
							lic.Email = &email
						}
						lic.Trial = &trial
						return nil
					},
				})
				if err != nil {
					return xerrors.Errorf("create telemetry reporter: %w", err)
				}
				defer options.Telemetry.Close()
			} else {
				logger.Warn(ctx, `telemetry disabled, unable to notify of security issues. Read more: https://coder.com/docs/v2/latest/admin/telemetry`)
			}

			// This prevents the pprof import from being accidentally deleted.
			_ = pprof.Handler
			if vals.Pprof.Enable {
				//nolint:revive
				defer ServeHandler(ctx, logger, nil, vals.Pprof.Address.String(), "pprof")()
			}
			if vals.Prometheus.Enable {
				closeFn, err := enablePrometheus(
					ctx,
					logger.Named("prometheus"),
					vals,
					options,
				)
				if err != nil {
					return xerrors.Errorf("enable prometheus: %w", err)
				}
				defer closeFn()
			}

			if vals.Swagger.Enable {
				options.SwaggerEndpoint = vals.Swagger.Enable.Value()
			}

			batcher, closeBatcher, err := batchstats.New(ctx,
				batchstats.WithLogger(options.Logger.Named("batchstats")),
				batchstats.WithStore(options.Database),
			)
			if err != nil {
				return xerrors.Errorf("failed to create agent stats batcher: %w", err)
			}
			options.StatsBatcher = batcher
			defer closeBatcher()

			// We use a separate coderAPICloser so the Enterprise API
			// can have its own close functions. This is cleaner
			// than abstracting the Coder API itself.
			coderAPI, coderAPICloser, err := newAPI(ctx, options)
			if err != nil {
				return xerrors.Errorf("create coder API: %w", err)
			}

			if vals.Prometheus.Enable {
				// Agent metrics require reference to the tailnet coordinator, so must be initiated after Coder API.
				closeAgentsFunc, err := prometheusmetrics.Agents(ctx, logger, options.PrometheusRegistry, coderAPI.Database, &coderAPI.TailnetCoordinator, coderAPI.DERPMap, coderAPI.Options.AgentInactiveDisconnectTimeout, 0)
				if err != nil {
					return xerrors.Errorf("register agents prometheus metric: %w", err)
				}
				defer closeAgentsFunc()

				var active codersdk.Experiments
				for _, exp := range options.DeploymentValues.Experiments.Value() {
					active = append(active, codersdk.Experiment(exp))
				}

				if err = prometheusmetrics.Experiments(options.PrometheusRegistry, active); err != nil {
					return xerrors.Errorf("register experiments metric: %w", err)
				}
			}

			client := codersdk.New(localURL)
			if localURL.Scheme == "https" && IsLocalhost(localURL.Hostname()) {
				// The certificate will likely be self-signed or for a different
				// hostname, so we need to skip verification.
				client.HTTPClient.Transport = &http.Transport{
					TLSClientConfig: &tls.Config{
						//nolint:gosec
						InsecureSkipVerify: true,
					},
				}
			}
			defer client.HTTPClient.CloseIdleConnections()

			// This is helpful for tests, but can be silently ignored.
			// Coder may be ran as users that don't have permission to write in the homedir,
			// such as via the systemd service.
			err = config.URL().Write(client.URL.String())
			if err != nil && flag.Lookup("test.v") != nil {
				return xerrors.Errorf("write config url: %w", err)
			}

			// Since errCh only has one buffered slot, all routines
			// sending on it must be wrapped in a select/default to
			// avoid leaving dangling goroutines waiting for the
			// channel to be consumed.
			errCh := make(chan error, 1)
			provisionerDaemons := make([]*provisionerd.Server, 0)
			defer func() {
				// We have no graceful shutdown of provisionerDaemons
				// here because that's handled at the end of main, this
				// is here in case the program exits early.
				for _, daemon := range provisionerDaemons {
					_ = daemon.Close()
				}
			}()

			var provisionerdWaitGroup sync.WaitGroup
			defer provisionerdWaitGroup.Wait()
			provisionerdMetrics := provisionerd.NewMetrics(options.PrometheusRegistry)
			for i := int64(0); i < vals.Provisioner.Daemons.Value(); i++ {
				suffix := fmt.Sprintf("%d", i)
				// The suffix is added to the hostname, so we may need to trim to fit into
				// the 64 character limit.
				hostname := stringutil.Truncate(cliutil.Hostname(), 63-len(suffix))
				name := fmt.Sprintf("%s-%s", hostname, suffix)
				daemonCacheDir := filepath.Join(cacheDir, fmt.Sprintf("provisioner-%d", i))
				daemon, err := newProvisionerDaemon(
					ctx, coderAPI, provisionerdMetrics, logger, vals, daemonCacheDir, errCh, &provisionerdWaitGroup, name,
				)
				if err != nil {
					return xerrors.Errorf("create provisioner daemon: %w", err)
				}
				provisionerDaemons = append(provisionerDaemons, daemon)
			}
			provisionerdMetrics.Runner.NumDaemons.Set(float64(len(provisionerDaemons)))

			shutdownConnsCtx, shutdownConns := context.WithCancel(ctx)
			defer shutdownConns()

			// Ensures that old database entries are cleaned up over time!
			purger := dbpurge.New(ctx, logger, options.Database)
			defer purger.Close()

			// Updates workspace usage
			tracker := workspaceusage.New(options.Database,
				workspaceusage.WithLogger(logger.Named("workspace_usage_tracker")),
			)
			options.WorkspaceUsageTracker = tracker
			defer tracker.Close()

			// Wrap the server in middleware that redirects to the access URL if
			// the request is not to a local IP.
			var handler http.Handler = coderAPI.RootHandler
			if vals.RedirectToAccessURL {
				handler = redirectToAccessURL(handler, vals.AccessURL.Value(), tunnel != nil, appHostnameRegex)
			}

			// ReadHeaderTimeout is purposefully not enabled. It caused some
			// issues with websockets over the dev tunnel.
			// See: https://github.com/coder/coder/pull/3730
			//nolint:gosec
			httpServer := &http.Server{
				// These errors are typically noise like "TLS: EOF". Vault does
				// similar:
				// https://github.com/hashicorp/vault/blob/e2490059d0711635e529a4efcbaa1b26998d6e1c/command/server.go#L2714
				ErrorLog: log.New(io.Discard, "", 0),
				Handler:  handler,
				BaseContext: func(_ net.Listener) context.Context {
					return shutdownConnsCtx
				},
			}
			defer func() {
				_ = shutdownWithTimeout(httpServer.Shutdown, 5*time.Second)
			}()

			// We call this in the routine so we can kill the other listeners if
			// one of them fails.
			closeListenersNow := func() {
				httpServers.Close()
				if tunnel != nil {
					_ = tunnel.Listener.Close()
				}
			}

			eg := errgroup.Group{}
			eg.Go(func() error {
				defer closeListenersNow()
				return httpServers.Serve(httpServer)
			})
			if tunnel != nil {
				eg.Go(func() error {
					defer closeListenersNow()
					return httpServer.Serve(tunnel.Listener)
				})
			}

			go func() {
				select {
				case errCh <- eg.Wait():
				default:
				}
			}()

			cliui.Infof(inv.Stdout, "\n==> Logs will stream in below (press ctrl+c to gracefully exit):")

			// Updates the systemd status from activating to activated.
			_, err = daemon.SdNotify(false, daemon.SdNotifyReady)
			if err != nil {
				return xerrors.Errorf("notify systemd: %w", err)
			}

			autobuildTicker := time.NewTicker(vals.AutobuildPollInterval.Value())
			defer autobuildTicker.Stop()
			autobuildExecutor := autobuild.NewExecutor(
				ctx, options.Database, options.Pubsub, coderAPI.TemplateScheduleStore, &coderAPI.Auditor, coderAPI.AccessControlStore, logger, autobuildTicker.C)
			autobuildExecutor.Run()

			hangDetectorTicker := time.NewTicker(vals.JobHangDetectorInterval.Value())
			defer hangDetectorTicker.Stop()
			hangDetector := unhanger.New(ctx, options.Database, options.Pubsub, logger, hangDetectorTicker.C)
			hangDetector.Start()
			defer hangDetector.Close()

			waitForProvisionerJobs := false
			// Currently there is no way to ask the server to shut
			// itself down, so any exit signal will result in a non-zero
			// exit of the server.
			var exitErr error
			select {
			case <-stopCtx.Done():
				exitErr = stopCtx.Err()
				waitForProvisionerJobs = true
				_, _ = io.WriteString(inv.Stdout, cliui.Bold("Stop caught, waiting for provisioner jobs to complete and gracefully exiting. Use ctrl+\\ to force quit"))
			case <-interruptCtx.Done():
				exitErr = interruptCtx.Err()
				_, _ = io.WriteString(inv.Stdout, cliui.Bold("Interrupt caught, gracefully exiting. Use ctrl+\\ to force quit"))
			case <-tunnelDone:
				exitErr = xerrors.New("dev tunnel closed unexpectedly")
			case <-pubsubWatchdogTimeout:
				exitErr = xerrors.New("pubsub Watchdog timed out")
			case exitErr = <-errCh:
			}
			if exitErr != nil && !xerrors.Is(exitErr, context.Canceled) {
				cliui.Errorf(inv.Stderr, "Unexpected error, shutting down server: %s\n", exitErr)
			}

			// Begin clean shut down stage, we try to shut down services
			// gracefully in an order that gives the best experience.
			// This procedure should not differ greatly from the order
			// of `defer`s in this function, but allows us to inform
			// the user about what's going on and handle errors more
			// explicitly.

			_, err = daemon.SdNotify(false, daemon.SdNotifyStopping)
			if err != nil {
				cliui.Errorf(inv.Stderr, "Notify systemd failed: %s", err)
			}

			// Stop accepting new connections without interrupting
			// in-flight requests, give in-flight requests 5 seconds to
			// complete.
			cliui.Info(inv.Stdout, "Shutting down API server..."+"\n")
			err = shutdownWithTimeout(httpServer.Shutdown, 3*time.Second)
			if err != nil {
				cliui.Errorf(inv.Stderr, "API server shutdown took longer than 3s: %s\n", err)
			} else {
				cliui.Info(inv.Stdout, "Gracefully shut down API server\n")
			}
			// Cancel any remaining in-flight requests.
			shutdownConns()

			// Shut down provisioners before waiting for WebSockets
			// connections to close.
			var wg sync.WaitGroup
			for i, provisionerDaemon := range provisionerDaemons {
				id := i + 1
				provisionerDaemon := provisionerDaemon
				wg.Add(1)
				go func() {
					defer wg.Done()

					r.Verbosef(inv, "Shutting down provisioner daemon %d...", id)
					timeout := 5 * time.Second
					if waitForProvisionerJobs {
						// It can last for a long time...
						timeout = 30 * time.Minute
					}

					err := shutdownWithTimeout(func(ctx context.Context) error {
						// We only want to cancel active jobs if we aren't exiting gracefully.
						return provisionerDaemon.Shutdown(ctx, !waitForProvisionerJobs)
					}, timeout)
					if err != nil {
						cliui.Errorf(inv.Stderr, "Failed to shut down provisioner daemon %d: %s\n", id, err)
						return
					}
					err = provisionerDaemon.Close()
					if err != nil {
						cliui.Errorf(inv.Stderr, "Close provisioner daemon %d: %s\n", id, err)
						return
					}
					r.Verbosef(inv, "Gracefully shut down provisioner daemon %d", id)
				}()
			}
			wg.Wait()

			cliui.Info(inv.Stdout, "Waiting for WebSocket connections to close..."+"\n")
			_ = coderAPICloser.Close()
			cliui.Info(inv.Stdout, "Done waiting for WebSocket connections"+"\n")

			// Close tunnel after we no longer have in-flight connections.
			if tunnel != nil {
				cliui.Infof(inv.Stdout, "Waiting for tunnel to close...")
				_ = tunnel.Close()
				<-tunnel.Wait()
				cliui.Infof(inv.Stdout, "Done waiting for tunnel")
			}

			// Ensures a last report can be sent before exit!
			options.Telemetry.Close()

			// Trigger context cancellation for any remaining services.
			cancel()

			switch {
			case xerrors.Is(exitErr, context.DeadlineExceeded):
				cliui.Warnf(inv.Stderr, "Graceful shutdown timed out")
				// Errors here cause a significant number of benign CI failures.
				return nil
			case xerrors.Is(exitErr, context.Canceled):
				return nil
			case exitErr != nil:
				return xerrors.Errorf("graceful shutdown: %w", exitErr)
			default:
				return nil
			}
		},
	}

	var pgRawURL bool

	postgresBuiltinURLCmd := &serpent.Command{
		Use:   "postgres-builtin-url",
		Short: "Output the connection URL for the built-in PostgreSQL deployment.",
		Handler: func(inv *serpent.Invocation) error {
			url, err := embeddedPostgresURL(r.createConfig())
			if err != nil {
				return err
			}
			if pgRawURL {
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", url)
			} else {
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", pretty.Sprint(cliui.DefaultStyles.Code, fmt.Sprintf("psql %q", url)))
			}
			return nil
		},
	}

	postgresBuiltinServeCmd := &serpent.Command{
		Use:   "postgres-builtin-serve",
		Short: "Run the built-in PostgreSQL deployment.",
		Handler: func(inv *serpent.Invocation) error {
			ctx := inv.Context()

			cfg := r.createConfig()
			logger := inv.Logger.AppendSinks(sloghuman.Sink(inv.Stderr))
			if ok, _ := inv.ParsedFlags().GetBool(varVerbose); ok {
				logger = logger.Leveled(slog.LevelDebug)
			}

			ctx, cancel := inv.SignalNotifyContext(ctx, InterruptSignals...)
			defer cancel()

			url, closePg, err := startBuiltinPostgres(ctx, cfg, logger)
			if err != nil {
				return err
			}
			defer func() { _ = closePg() }()

			if pgRawURL {
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", url)
			} else {
				_, _ = fmt.Fprintf(inv.Stdout, "%s\n", pretty.Sprint(cliui.DefaultStyles.Code, fmt.Sprintf("psql %q", url)))
			}

			<-ctx.Done()
			return nil
		},
	}

	createAdminUserCmd := r.newCreateAdminUserCommand()

	rawURLOpt := serpent.Option{
		Flag: "raw-url",

		Value:       serpent.BoolOf(&pgRawURL),
		Description: "Output the raw connection URL instead of a psql command.",
	}
	createAdminUserCmd.Options.Add(rawURLOpt)
	postgresBuiltinURLCmd.Options.Add(rawURLOpt)
	postgresBuiltinServeCmd.Options.Add(rawURLOpt)

	serverCmd.Children = append(
		serverCmd.Children,
		createAdminUserCmd, postgresBuiltinURLCmd, postgresBuiltinServeCmd,
	)

	return serverCmd
}

// printDeprecatedOptions loops through all command options, and prints
// a warning for usage of deprecated options.
func PrintDeprecatedOptions() serpent.MiddlewareFunc {
	return func(next serpent.HandlerFunc) serpent.HandlerFunc {
		return func(inv *serpent.Invocation) error {
			opts := inv.Command.Options
			// Print deprecation warnings.
			for _, opt := range opts {
				if opt.UseInstead == nil {
					continue
				}

				if opt.ValueSource == serpent.ValueSourceNone || opt.ValueSource == serpent.ValueSourceDefault {
					continue
				}

				warnStr := opt.Name + " is deprecated, please use "
				for i, use := range opt.UseInstead {
					warnStr += use.Name + " "
					if i != len(opt.UseInstead)-1 {
						warnStr += "and "
					}
				}
				warnStr += "instead.\n"

				cliui.Warn(inv.Stderr,
					warnStr,
				)
			}

			return next(inv)
		}
	}
}

// writeConfigMW will prevent the main command from running if the write-config
// flag is set. Instead, it will marshal the command options to YAML and write
// them to stdout.
func WriteConfigMW(cfg *codersdk.DeploymentValues) serpent.MiddlewareFunc {
	return func(next serpent.HandlerFunc) serpent.HandlerFunc {
		return func(inv *serpent.Invocation) error {
			if !cfg.WriteConfig {
				return next(inv)
			}

			opts := inv.Command.Options
			n, err := opts.MarshalYAML()
			if err != nil {
				return xerrors.Errorf("generate yaml: %w", err)
			}
			enc := yaml.NewEncoder(inv.Stdout)
			enc.SetIndent(2)
			err = enc.Encode(n)
			if err != nil {
				return xerrors.Errorf("encode yaml: %w", err)
			}
			err = enc.Close()
			if err != nil {
				return xerrors.Errorf("close yaml encoder: %w", err)
			}
			return nil
		}
	}
}

// isLocalURL returns true if the hostname of the provided URL appears to
// resolve to a loopback address.
func IsLocalURL(ctx context.Context, u *url.URL) (bool, error) {
	// In tests, we commonly use "example.com" or "google.com", which
	// are not loopback, so avoid the DNS lookup to avoid flakes.
	if flag.Lookup("test.v") != nil {
		if u.Hostname() == "example.com" || u.Hostname() == "google.com" {
			return false, nil
		}
	}

	resolver := &net.Resolver{}
	ips, err := resolver.LookupIPAddr(ctx, u.Hostname())
	if err != nil {
		return false, err
	}

	for _, ip := range ips {
		if ip.IP.IsLoopback() {
			return true, nil
		}
	}
	return false, nil
}

func shutdownWithTimeout(shutdown func(context.Context) error, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return shutdown(ctx)
}

// nolint:revive
func newProvisionerDaemon(
	ctx context.Context,
	coderAPI *coderd.API,
	metrics provisionerd.Metrics,
	logger slog.Logger,
	cfg *codersdk.DeploymentValues,
	cacheDir string,
	errCh chan error,
	wg *sync.WaitGroup,
	name string,
) (srv *provisionerd.Server, err error) {
	ctx, cancel := context.WithCancel(ctx)
	defer func() {
		if err != nil {
			cancel()
		}
	}()

	err = os.MkdirAll(cacheDir, 0o700)
	if err != nil {
		return nil, xerrors.Errorf("mkdir %q: %w", cacheDir, err)
	}

	workDir := filepath.Join(cacheDir, "work")
	err = os.MkdirAll(workDir, 0o700)
	if err != nil {
		return nil, xerrors.Errorf("mkdir work dir: %w", err)
	}

	connector := provisionerd.LocalProvisioners{}
	if cfg.Provisioner.DaemonsEcho {
		echoClient, echoServer := drpc.MemTransportPipe()
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ctx.Done()
			_ = echoClient.Close()
			_ = echoServer.Close()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()

			err := echo.Serve(ctx, &provisionersdk.ServeOptions{
				Listener:      echoServer,
				WorkDirectory: workDir,
				Logger:        logger.Named("echo"),
			})
			if err != nil {
				select {
				case errCh <- err:
				default:
				}
			}
		}()
		connector[string(database.ProvisionerTypeEcho)] = sdkproto.NewDRPCProvisionerClient(echoClient)
	} else {
		tfDir := filepath.Join(cacheDir, "tf")
		err = os.MkdirAll(tfDir, 0o700)
		if err != nil {
			return nil, xerrors.Errorf("mkdir terraform dir: %w", err)
		}

		tracer := coderAPI.TracerProvider.Tracer(tracing.TracerName)
		terraformClient, terraformServer := drpc.MemTransportPipe()
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-ctx.Done()
			_ = terraformClient.Close()
			_ = terraformServer.Close()
		}()
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer cancel()

			err := terraform.Serve(ctx, &terraform.ServeOptions{
				ServeOptions: &provisionersdk.ServeOptions{
					Listener:      terraformServer,
					Logger:        logger.Named("terraform"),
					WorkDirectory: workDir,
				},
				CachePath: tfDir,
				Tracer:    tracer,
			})
			if err != nil && !xerrors.Is(err, context.Canceled) {
				select {
				case errCh <- err:
				default:
				}
			}
		}()

		connector[string(database.ProvisionerTypeTerraform)] = sdkproto.NewDRPCProvisionerClient(terraformClient)
	}

	return provisionerd.New(func(dialCtx context.Context) (proto.DRPCProvisionerDaemonClient, error) {
		// This debounces calls to listen every second. Read the comment
		// in provisionerdserver.go to learn more!
		return coderAPI.CreateInMemoryProvisionerDaemon(dialCtx, name)
	}, &provisionerd.Options{
		Logger:              logger.Named(fmt.Sprintf("provisionerd-%s", name)),
		UpdateInterval:      time.Second,
		ForceCancelInterval: cfg.Provisioner.ForceCancelInterval.Value(),
		Connector:           connector,
		TracerProvider:      coderAPI.TracerProvider,
		Metrics:             &metrics,
	}), nil
}

// nolint: revive
func PrintLogo(inv *serpent.Invocation, daemonTitle string) {
	// Only print the logo in TTYs.
	if !isTTYOut(inv) {
		return
	}

	versionString := cliui.Bold(daemonTitle + " " + buildinfo.Version())

	_, _ = fmt.Fprintf(inv.Stdout, "%s - Your Self-Hosted Remote Development Platform\n", versionString)
}

func loadCertificates(tlsCertFiles, tlsKeyFiles []string) ([]tls.Certificate, error) {
	if len(tlsCertFiles) != len(tlsKeyFiles) {
		return nil, xerrors.New("--tls-cert-file and --tls-key-file must be used the same amount of times")
	}

	certs := make([]tls.Certificate, len(tlsCertFiles))
	for i := range tlsCertFiles {
		certFile, keyFile := tlsCertFiles[i], tlsKeyFiles[i]
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, xerrors.Errorf(
				"load TLS key pair %d (%q, %q): %w\ncertFiles: %+v\nkeyFiles: %+v",
				i, certFile, keyFile, err,
				tlsCertFiles, tlsKeyFiles,
			)
		}

		certs[i] = cert
	}

	return certs, nil
}

// generateSelfSignedCertificate creates an unsafe self-signed certificate
// at random that allows users to proceed with setup in the event they
// haven't configured any TLS certificates.
func generateSelfSignedCertificate() (*tls.Certificate, error) {
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour * 24 * 180),

		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}

	derBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, err
	}

	var cert tls.Certificate
	cert.Certificate = append(cert.Certificate, derBytes)
	cert.PrivateKey = privateKey
	return &cert, nil
}

// configureServerTLS returns the TLS config used for the Coderd server
// connections to clients. A logger is passed in to allow printing warning
// messages that do not block startup.
//
//nolint:revive
func configureServerTLS(ctx context.Context, logger slog.Logger, tlsMinVersion, tlsClientAuth string, tlsCertFiles, tlsKeyFiles []string, tlsClientCAFile string, ciphers []string, allowInsecureCiphers bool) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
		NextProtos: []string{"h2", "http/1.1"},
	}
	switch tlsMinVersion {
	case "tls10":
		tlsConfig.MinVersion = tls.VersionTLS10
	case "tls11":
		tlsConfig.MinVersion = tls.VersionTLS11
	case "tls12":
		tlsConfig.MinVersion = tls.VersionTLS12
	case "tls13":
		tlsConfig.MinVersion = tls.VersionTLS13
	default:
		return nil, xerrors.Errorf("unrecognized tls version: %q", tlsMinVersion)
	}

	// A custom set of supported ciphers.
	if len(ciphers) > 0 {
		cipherIDs, err := configureCipherSuites(ctx, logger, ciphers, allowInsecureCiphers, tlsConfig.MinVersion, tls.VersionTLS13)
		if err != nil {
			return nil, err
		}
		tlsConfig.CipherSuites = cipherIDs
	}

	switch tlsClientAuth {
	case "none":
		tlsConfig.ClientAuth = tls.NoClientCert
	case "request":
		tlsConfig.ClientAuth = tls.RequestClientCert
	case "require-any":
		tlsConfig.ClientAuth = tls.RequireAnyClientCert
	case "verify-if-given":
		tlsConfig.ClientAuth = tls.VerifyClientCertIfGiven
	case "require-and-verify":
		tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
	default:
		return nil, xerrors.Errorf("unrecognized tls client auth: %q", tlsClientAuth)
	}

	certs, err := loadCertificates(tlsCertFiles, tlsKeyFiles)
	if err != nil {
		return nil, xerrors.Errorf("load certificates: %w", err)
	}
	if len(certs) == 0 {
		selfSignedCertificate, err := generateSelfSignedCertificate()
		if err != nil {
			return nil, xerrors.Errorf("generate self signed certificate: %w", err)
		}
		certs = append(certs, *selfSignedCertificate)
	}

	tlsConfig.Certificates = certs
	tlsConfig.GetCertificate = func(hi *tls.ClientHelloInfo) (*tls.Certificate, error) {
		// If there's only one certificate, return it.
		if len(certs) == 1 {
			return &certs[0], nil
		}

		// Expensively check which certificate matches the client hello.
		for _, cert := range certs {
			cert := cert
			if err := hi.SupportsCertificate(&cert); err == nil {
				return &cert, nil
			}
		}

		// Return the first certificate if we have one, or return nil so the
		// server doesn't fail.
		if len(certs) > 0 {
			return &certs[0], nil
		}
		return nil, nil //nolint:nilnil
	}

	err = configureCAPool(tlsClientCAFile, tlsConfig)
	if err != nil {
		return nil, err
	}

	return tlsConfig, nil
}

//nolint:revive
func configureCipherSuites(ctx context.Context, logger slog.Logger, ciphers []string, allowInsecureCiphers bool, minTLS, maxTLS uint16) ([]uint16, error) {
	if minTLS > maxTLS {
		return nil, xerrors.Errorf("minimum tls version (%s) cannot be greater than maximum tls version (%s)", versionName(minTLS), versionName(maxTLS))
	}
	if minTLS >= tls.VersionTLS13 {
		// The cipher suites config option is ignored for tls 1.3 and higher.
		// So this user flag is a no-op if the min version is 1.3.
		return nil, xerrors.Errorf("'--tls-ciphers' cannot be specified when using minimum tls version 1.3 or higher, %d ciphers found as input.", len(ciphers))
	}
	// Configure the cipher suites which parses the strings and converts them
	// to golang cipher suites.
	supported, err := parseTLSCipherSuites(ciphers)
	if err != nil {
		return nil, xerrors.Errorf("tls ciphers: %w", err)
	}

	// allVersions is all tls versions the server supports.
	// We enumerate these to ensure if ciphers are configured, at least
	// 1 cipher for each version exists.
	allVersions := make(map[uint16]bool)
	for v := minTLS; v <= maxTLS; v++ {
		allVersions[v] = false
	}

	var insecure []string
	cipherIDs := make([]uint16, 0, len(supported))
	for _, cipher := range supported {
		if cipher.Insecure {
			// Always show this warning, even if they have allowInsecureCiphers
			// specified.
			logger.Warn(ctx, "insecure tls cipher specified for server use", slog.F("cipher", cipher.Name))
			insecure = append(insecure, cipher.Name)
		}

		// This is a warning message to tell the user if they are specifying
		// a cipher that does not support the tls versions they have specified.
		// This makes the cipher essentially a "noop" cipher.
		if !hasSupportedVersion(minTLS, maxTLS, cipher.SupportedVersions) {
			versions := make([]string, 0, len(cipher.SupportedVersions))
			for _, sv := range cipher.SupportedVersions {
				versions = append(versions, versionName(sv))
			}
			logger.Warn(ctx, "cipher not supported for tls versions enabled, cipher will not be used",
				slog.F("cipher", cipher.Name),
				slog.F("cipher_supported_versions", strings.Join(versions, ",")),
				slog.F("server_min_version", versionName(minTLS)),
				slog.F("server_max_version", versionName(maxTLS)),
			)
		}

		for _, v := range cipher.SupportedVersions {
			allVersions[v] = true
		}

		cipherIDs = append(cipherIDs, cipher.ID)
	}

	if len(insecure) > 0 && !allowInsecureCiphers {
		return nil, xerrors.Errorf("insecure tls ciphers specified, must use '--tls-allow-insecure-ciphers' to allow these: %s", strings.Join(insecure, ", "))
	}

	// This is an additional sanity check. The user can specify ciphers that
	// do not cover the full range of tls versions they have specified.
	// They can unintentionally break TLS for some tls configured versions.
	var missedVersions []string
	for version, covered := range allVersions {
		if version == tls.VersionTLS13 {
			continue // v1.3 ignores configured cipher suites.
		}
		if !covered {
			missedVersions = append(missedVersions, versionName(version))
		}
	}
	if len(missedVersions) > 0 {
		return nil, xerrors.Errorf("no tls ciphers supported for tls versions %q."+
			"Add additional ciphers, set the minimum version to 'tls13, or remove the ciphers configured and rely on the default",
			strings.Join(missedVersions, ","))
	}

	return cipherIDs, nil
}

// parseTLSCipherSuites will parse cipher suite names like 'TLS_RSA_WITH_AES_128_CBC_SHA'
// to their tls cipher suite structs. If a cipher suite that is unsupported is
// passed in, this function will return an error.
// This function can return insecure cipher suites.
func parseTLSCipherSuites(ciphers []string) ([]tls.CipherSuite, error) {
	if len(ciphers) == 0 {
		return nil, nil
	}

	var unsupported []string
	var supported []tls.CipherSuite
	// A custom set of supported ciphers.
	allCiphers := append(tls.CipherSuites(), tls.InsecureCipherSuites()...)
	for _, cipher := range ciphers {
		// For each cipher specified by the client, find the cipher in the
		// list of golang supported ciphers.
		var found *tls.CipherSuite
		for _, supported := range allCiphers {
			if strings.EqualFold(supported.Name, cipher) {
				found = supported
				break
			}
		}

		if found == nil {
			unsupported = append(unsupported, cipher)
			continue
		}

		supported = append(supported, *found)
	}

	if len(unsupported) > 0 {
		return nil, xerrors.Errorf("unsupported tls ciphers specified, see https://github.com/golang/go/blob/master/src/crypto/tls/cipher_suites.go#L53-L75: %s", strings.Join(unsupported, ", "))
	}

	return supported, nil
}

// hasSupportedVersion is a helper function that returns true if the list
// of supported versions contains a version between min and max.
// If the versions list is outside the min/max, then it returns false.
func hasSupportedVersion(min, max uint16, versions []uint16) bool {
	for _, v := range versions {
		if v >= min && v <= max {
			// If one version is in between min/max, return true.
			return true
		}
	}
	return false
}

// versionName is tls.VersionName in go 1.21.
// Until the switch, the function is copied locally.
func versionName(version uint16) string {
	switch version {
	case tls.VersionSSL30:
		return "SSLv3"
	case tls.VersionTLS10:
		return "TLS 1.0"
	case tls.VersionTLS11:
		return "TLS 1.1"
	case tls.VersionTLS12:
		return "TLS 1.2"
	case tls.VersionTLS13:
		return "TLS 1.3"
	default:
		return fmt.Sprintf("0x%04X", version)
	}
}

func configureOIDCPKI(orig *oauth2.Config, keyFile string, certFile string) (*oauthpki.Config, error) {
	// Read the files
	keyData, err := os.ReadFile(keyFile)
	if err != nil {
		return nil, xerrors.Errorf("read oidc client key file: %w", err)
	}

	var certData []byte
	// According to the spec, this is not required. So do not require it on the initial loading
	// of the PKI config.
	if certFile != "" {
		certData, err = os.ReadFile(certFile)
		if err != nil {
			return nil, xerrors.Errorf("read oidc client cert file: %w", err)
		}
	}

	return oauthpki.NewOauth2PKIConfig(oauthpki.ConfigParams{
		ClientID:       orig.ClientID,
		TokenURL:       orig.Endpoint.TokenURL,
		Scopes:         orig.Scopes,
		PemEncodedKey:  keyData,
		PemEncodedCert: certData,
		Config:         orig,
	})
}

func configureCAPool(tlsClientCAFile string, tlsConfig *tls.Config) error {
	if tlsClientCAFile != "" {
		caPool := x509.NewCertPool()
		data, err := os.ReadFile(tlsClientCAFile)
		if err != nil {
			return xerrors.Errorf("read %q: %w", tlsClientCAFile, err)
		}
		if !caPool.AppendCertsFromPEM(data) {
			return xerrors.Errorf("failed to parse CA certificate in tls-client-ca-file")
		}
		tlsConfig.ClientCAs = caPool
	}
	return nil
}

//nolint:revive // Ignore flag-parameter: parameter 'allowEveryone' seems to be a control flag, avoid control coupling (revive)
func configureGithubOAuth2(instrument *promoauth.Factory, accessURL *url.URL, clientID, clientSecret string, allowSignups, allowEveryone bool, allowOrgs []string, rawTeams []string, enterpriseBaseURL string) (*coderd.GithubOAuth2Config, error) {
	redirectURL, err := accessURL.Parse("/api/v2/users/oauth2/github/callback")
	if err != nil {
		return nil, xerrors.Errorf("parse github oauth callback url: %w", err)
	}
	if allowEveryone && len(allowOrgs) > 0 {
		return nil, xerrors.New("allow everyone and allowed orgs cannot be used together")
	}
	if allowEveryone && len(rawTeams) > 0 {
		return nil, xerrors.New("allow everyone and allowed teams cannot be used together")
	}
	if !allowEveryone && len(allowOrgs) == 0 {
		return nil, xerrors.New("allowed orgs is empty: must specify at least one org or allow everyone")
	}
	allowTeams := make([]coderd.GithubOAuth2Team, 0, len(rawTeams))
	for _, rawTeam := range rawTeams {
		parts := strings.SplitN(rawTeam, "/", 2)
		if len(parts) != 2 {
			return nil, xerrors.Errorf("github team allowlist is formatted incorrectly. got %s; wanted <organization>/<team>", rawTeam)
		}
		allowTeams = append(allowTeams, coderd.GithubOAuth2Team{
			Organization: parts[0],
			Slug:         parts[1],
		})
	}

	endpoint := xgithub.Endpoint
	if enterpriseBaseURL != "" {
		enterpriseURL, err := url.Parse(enterpriseBaseURL)
		if err != nil {
			return nil, xerrors.Errorf("parse enterprise base url: %w", err)
		}
		authURL, err := enterpriseURL.Parse("/login/oauth/authorize")
		if err != nil {
			return nil, xerrors.Errorf("parse enterprise auth url: %w", err)
		}
		tokenURL, err := enterpriseURL.Parse("/login/oauth/access_token")
		if err != nil {
			return nil, xerrors.Errorf("parse enterprise token url: %w", err)
		}
		endpoint = oauth2.Endpoint{
			AuthURL:  authURL.String(),
			TokenURL: tokenURL.String(),
		}
	}

	instrumentedOauth := instrument.NewGithub("github-login", &oauth2.Config{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		Endpoint:     endpoint,
		RedirectURL:  redirectURL.String(),
		Scopes: []string{
			"read:user",
			"read:org",
			"user:email",
		},
	})

	createClient := func(client *http.Client, source promoauth.Oauth2Source) (*github.Client, error) {
		client = instrumentedOauth.InstrumentHTTPClient(client, source)
		if enterpriseBaseURL != "" {
			return github.NewEnterpriseClient(enterpriseBaseURL, "", client)
		}
		return github.NewClient(client), nil
	}

	return &coderd.GithubOAuth2Config{
		OAuth2Config:       instrumentedOauth,
		AllowSignups:       allowSignups,
		AllowEveryone:      allowEveryone,
		AllowOrganizations: allowOrgs,
		AllowTeams:         allowTeams,
		AuthenticatedUser: func(ctx context.Context, client *http.Client) (*github.User, error) {
			api, err := createClient(client, promoauth.SourceGitAPIAuthUser)
			if err != nil {
				return nil, err
			}
			user, _, err := api.Users.Get(ctx, "")
			return user, err
		},
		ListEmails: func(ctx context.Context, client *http.Client) ([]*github.UserEmail, error) {
			api, err := createClient(client, promoauth.SourceGitAPIListEmails)
			if err != nil {
				return nil, err
			}
			emails, _, err := api.Users.ListEmails(ctx, &github.ListOptions{})
			return emails, err
		},
		ListOrganizationMemberships: func(ctx context.Context, client *http.Client) ([]*github.Membership, error) {
			api, err := createClient(client, promoauth.SourceGitAPIOrgMemberships)
			if err != nil {
				return nil, err
			}
			memberships, _, err := api.Organizations.ListOrgMemberships(ctx, &github.ListOrgMembershipsOptions{
				State: "active",
				ListOptions: github.ListOptions{
					PerPage: 100,
				},
			})
			return memberships, err
		},
		TeamMembership: func(ctx context.Context, client *http.Client, org, teamSlug, username string) (*github.Membership, error) {
			api, err := createClient(client, promoauth.SourceGitAPITeamMemberships)
			if err != nil {
				return nil, err
			}
			team, _, err := api.Teams.GetTeamMembershipBySlug(ctx, org, teamSlug, username)
			return team, err
		},
	}, nil
}

// embeddedPostgresURL returns the URL for the embedded PostgreSQL deployment.
func embeddedPostgresURL(cfg config.Root) (string, error) {
	pgPassword, err := cfg.PostgresPassword().Read()
	if errors.Is(err, os.ErrNotExist) {
		pgPassword, err = cryptorand.String(16)
		if err != nil {
			return "", xerrors.Errorf("generate password: %w", err)
		}
		err = cfg.PostgresPassword().Write(pgPassword)
		if err != nil {
			return "", xerrors.Errorf("write password: %w", err)
		}
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	pgPort, err := cfg.PostgresPort().Read()
	if errors.Is(err, os.ErrNotExist) {
		listener, err := net.Listen("tcp4", "127.0.0.1:0")
		if err != nil {
			return "", xerrors.Errorf("listen for random port: %w", err)
		}
		_ = listener.Close()
		tcpAddr, valid := listener.Addr().(*net.TCPAddr)
		if !valid {
			return "", xerrors.Errorf("listener returned non TCP addr: %T", tcpAddr)
		}
		pgPort = strconv.Itoa(tcpAddr.Port)
		err = cfg.PostgresPort().Write(pgPort)
		if err != nil {
			return "", xerrors.Errorf("write postgres port: %w", err)
		}
	}
	return fmt.Sprintf("postgres://coder@localhost:%s/coder?sslmode=disable&password=%s", pgPort, pgPassword), nil
}

func startBuiltinPostgres(ctx context.Context, cfg config.Root, logger slog.Logger) (string, func() error, error) {
	usr, err := user.Current()
	if err != nil {
		return "", nil, err
	}
	if usr.Uid == "0" {
		return "", nil, xerrors.New("The built-in PostgreSQL cannot run as the root user. Create a non-root user and run again!")
	}

	// Ensure a password and port have been generated!
	connectionURL, err := embeddedPostgresURL(cfg)
	if err != nil {
		return "", nil, err
	}
	pgPassword, err := cfg.PostgresPassword().Read()
	if err != nil {
		return "", nil, xerrors.Errorf("read postgres password: %w", err)
	}
	pgPortRaw, err := cfg.PostgresPort().Read()
	if err != nil {
		return "", nil, xerrors.Errorf("read postgres port: %w", err)
	}
	pgPort, err := strconv.ParseUint(pgPortRaw, 10, 16)
	if err != nil {
		return "", nil, xerrors.Errorf("parse postgres port: %w", err)
	}

	stdlibLogger := slog.Stdlib(ctx, logger.Named("postgres"), slog.LevelDebug)
	ep := embeddedpostgres.NewDatabase(
		embeddedpostgres.DefaultConfig().
			Version(embeddedpostgres.V13).
			BinariesPath(filepath.Join(cfg.PostgresPath(), "bin")).
			DataPath(filepath.Join(cfg.PostgresPath(), "data")).
			RuntimePath(filepath.Join(cfg.PostgresPath(), "runtime")).
			CachePath(filepath.Join(cfg.PostgresPath(), "cache")).
			Username("coder").
			Password(pgPassword).
			Database("coder").
			Port(uint32(pgPort)).
			Logger(stdlibLogger.Writer()),
	)
	err = ep.Start()
	if err != nil {
		return "", nil, xerrors.Errorf("Failed to start built-in PostgreSQL. Optionally, specify an external deployment with `--postgres-url`: %w", err)
	}
	return connectionURL, ep.Stop, nil
}

func ConfigureHTTPClient(ctx context.Context, clientCertFile, clientKeyFile string, tlsClientCAFile string) (context.Context, *http.Client, error) {
	if clientCertFile != "" && clientKeyFile != "" {
		certificates, err := loadCertificates([]string{clientCertFile}, []string{clientKeyFile})
		if err != nil {
			return ctx, nil, err
		}

		tlsClientConfig := &tls.Config{ //nolint:gosec
			Certificates: certificates,
			NextProtos:   []string{"h2", "http/1.1"},
		}
		err = configureCAPool(tlsClientCAFile, tlsClientConfig)
		if err != nil {
			return nil, nil, err
		}

		httpClient := &http.Client{
			Transport: &http.Transport{
				TLSClientConfig: tlsClientConfig,
			},
		}
		return context.WithValue(ctx, oauth2.HTTPClient, httpClient), httpClient, nil
	}
	return ctx, &http.Client{}, nil
}

// nolint:revive
func redirectToAccessURL(handler http.Handler, accessURL *url.URL, tunnel bool, appHostnameRegex *regexp.Regexp) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirect := func() {
			http.Redirect(w, r, accessURL.String(), http.StatusTemporaryRedirect)
		}

		// Exception: /healthz
		// Kubernetes doesn't like it if you redirect your healthcheck or liveness check endpoint.
		if r.URL.Path == "/healthz" {
			handler.ServeHTTP(w, r)
			return
		}

		// Exception: DERP
		// We use this endpoint when creating a DERP-mesh in the enterprise version to directly
		// dial other Coderd derpers.  Redirecting to the access URL breaks direct dial since the
		// access URL will be load-balanced in a multi-replica deployment.
		//
		// It's totally fine to access DERP over TLS, but we also don't need to redirect HTTP to
		// HTTPS as DERP is itself an encrypted protocol.
		if isDERPPath(r.URL.Path) {
			handler.ServeHTTP(w, r)
			return
		}

		// Only do this if we aren't tunneling.
		// If we are tunneling, we want to allow the request to go through
		// because the tunnel doesn't proxy with TLS.
		if !tunnel && accessURL.Scheme == "https" && r.TLS == nil {
			redirect()
			return
		}

		if r.Host == accessURL.Host {
			handler.ServeHTTP(w, r)
			return
		}

		if r.Header.Get("X-Forwarded-Host") == accessURL.Host {
			handler.ServeHTTP(w, r)
			return
		}

		if appHostnameRegex != nil && appHostnameRegex.MatchString(r.Host) {
			handler.ServeHTTP(w, r)
			return
		}

		redirect()
	})
}

func isDERPPath(p string) bool {
	segments := strings.SplitN(p, "/", 3)
	if len(segments) < 2 {
		return false
	}
	return segments[1] == "derp"
}

// IsLocalhost returns true if the host points to the local machine. Intended to
// be called with `u.Hostname()`.
func IsLocalhost(host string) bool {
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}

func ConnectToPostgres(ctx context.Context, logger slog.Logger, driver string, dbURL string) (sqlDB *sql.DB, err error) {
	logger.Debug(ctx, "connecting to postgresql")

	// Try to connect for 30 seconds.
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	defer func() {
		if err == nil {
			return
		}
		if sqlDB != nil {
			_ = sqlDB.Close()
			sqlDB = nil
		}
		logger.Error(ctx, "connect to postgres failed", slog.Error(err))
	}()

	var tries int
	for r := retry.New(time.Second, 3*time.Second); r.Wait(ctx); {
		tries++

		sqlDB, err = sql.Open(driver, dbURL)
		if err != nil {
			logger.Warn(ctx, "connect to postgres: retrying", slog.Error(err), slog.F("try", tries))
			continue
		}

		err = pingPostgres(ctx, sqlDB)
		if err != nil {
			logger.Warn(ctx, "ping postgres: retrying", slog.Error(err), slog.F("try", tries))
			_ = sqlDB.Close()
			sqlDB = nil
			continue
		}

		break
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		return nil, xerrors.Errorf("unable to connect after %d tries; last error: %w", tries, err)
	}

	// Ensure the PostgreSQL version is >=13.0.0!
	version, err := sqlDB.QueryContext(ctx, "SHOW server_version_num;")
	if err != nil {
		return nil, xerrors.Errorf("get postgres version: %w", err)
	}
	if !version.Next() {
		return nil, xerrors.Errorf("no rows returned for version select")
	}
	var versionNum int
	err = version.Scan(&versionNum)
	if err != nil {
		return nil, xerrors.Errorf("scan version: %w", err)
	}
	_ = version.Close()

	if versionNum < 130000 {
		return nil, xerrors.Errorf("PostgreSQL version must be v13.0.0 or higher! Got: %d", versionNum)
	}
	logger.Debug(ctx, "connected to postgresql", slog.F("version", versionNum))

	err = migrations.Up(sqlDB)
	if err != nil {
		return nil, xerrors.Errorf("migrate up: %w", err)
	}
	// The default is 0 but the request will fail with a 500 if the DB
	// cannot accept new connections, so we try to limit that here.
	// Requests will wait for a new connection instead of a hard error
	// if a limit is set.
	sqlDB.SetMaxOpenConns(10)
	// Allow a max of 3 idle connections at a time. Lower values end up
	// creating a lot of connection churn. Since each connection uses about
	// 10MB of memory, we're allocating 30MB to Postgres connections per
	// replica, but is better than causing Postgres to spawn a thread 15-20
	// times/sec. PGBouncer's transaction pooling is not the greatest so
	// it's not optimal for us to deploy.
	//
	// This was set to 10 before we started doing HA deployments, but 3 was
	// later determined to be a better middle ground as to not use up all
	// of PGs default connection limit while simultaneously avoiding a lot
	// of connection churn.
	sqlDB.SetMaxIdleConns(3)

	return sqlDB, nil
}

func pingPostgres(ctx context.Context, db *sql.DB) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return db.PingContext(ctx)
}

type HTTPServers struct {
	HTTPUrl      *url.URL
	HTTPListener net.Listener

	// TLS
	TLSUrl      *url.URL
	TLSListener net.Listener
	TLSConfig   *tls.Config
}

// Serve acts just like http.Serve. It is a blocking call until the server
// is closed, and an error is returned if any underlying Serve call fails.
func (s *HTTPServers) Serve(srv *http.Server) error {
	eg := errgroup.Group{}
	if s.HTTPListener != nil {
		eg.Go(func() error {
			defer s.Close() // close all listeners on error
			return srv.Serve(s.HTTPListener)
		})
	}
	if s.TLSListener != nil {
		eg.Go(func() error {
			defer s.Close() // close all listeners on error
			return srv.Serve(s.TLSListener)
		})
	}
	return eg.Wait()
}

func (s *HTTPServers) Close() {
	if s.HTTPListener != nil {
		_ = s.HTTPListener.Close()
	}
	if s.TLSListener != nil {
		_ = s.TLSListener.Close()
	}
}

func ConfigureTraceProvider(
	ctx context.Context,
	logger slog.Logger,
	cfg *codersdk.DeploymentValues,
) (trace.TracerProvider, string, func(context.Context) error) {
	var (
		tracerProvider = trace.NewNoopTracerProvider()
		closeTracing   = func(context.Context) error { return nil }
		sqlDriver      = "postgres"
	)

	otel.SetTextMapPropagator(
		propagation.NewCompositeTextMapPropagator(
			propagation.TraceContext{},
			propagation.Baggage{},
		),
	)

	if cfg.Trace.Enable.Value() || cfg.Trace.DataDog.Value() || cfg.Trace.HoneycombAPIKey != "" {
		sdkTracerProvider, _closeTracing, err := tracing.TracerProvider(ctx, "coderd", tracing.TracerOpts{
			Default:   cfg.Trace.Enable.Value(),
			DataDog:   cfg.Trace.DataDog.Value(),
			Honeycomb: cfg.Trace.HoneycombAPIKey.String(),
		})
		if err != nil {
			logger.Warn(ctx, "start telemetry exporter", slog.Error(err))
		} else {
			d, err := tracing.PostgresDriver(sdkTracerProvider, "coderd.database")
			if err != nil {
				logger.Warn(ctx, "start postgres tracing driver", slog.Error(err))
			} else {
				sqlDriver = d
			}

			tracerProvider = sdkTracerProvider
			closeTracing = _closeTracing
		}
	}
	return tracerProvider, sqlDriver, closeTracing
}

func ConfigureHTTPServers(logger slog.Logger, inv *serpent.Invocation, cfg *codersdk.DeploymentValues) (_ *HTTPServers, err error) {
	ctx := inv.Context()
	httpServers := &HTTPServers{}
	defer func() {
		if err != nil {
			// Always close the listeners if we fail.
			httpServers.Close()
		}
	}()
	// Validate bind addresses.
	if cfg.Address.String() != "" {
		if cfg.TLS.Enable {
			cfg.HTTPAddress = ""
			cfg.TLS.Address = cfg.Address
		} else {
			_ = cfg.HTTPAddress.Set(cfg.Address.String())
			cfg.TLS.Address.Host = ""
			cfg.TLS.Address.Port = ""
		}
	}
	if cfg.TLS.Enable && cfg.TLS.Address.String() == "" {
		return nil, xerrors.Errorf("TLS address must be set if TLS is enabled")
	}
	if !cfg.TLS.Enable && cfg.HTTPAddress.String() == "" {
		return nil, xerrors.Errorf("TLS is disabled. Enable with --tls-enable or specify a HTTP address")
	}

	if cfg.AccessURL.String() != "" &&
		!(cfg.AccessURL.Scheme == "http" || cfg.AccessURL.Scheme == "https") {
		return nil, xerrors.Errorf("access-url must include a scheme (e.g. 'http://' or 'https://)")
	}

	addrString := func(l net.Listener) string {
		listenAddrStr := l.Addr().String()
		// For some reason if 0.0.0.0:x is provided as the https
		// address, httpsListener.Addr().String() likes to return it as
		// an ipv6 address (i.e. [::]:x). If the input ip is 0.0.0.0,
		// try to coerce the output back to ipv4 to make it less
		// confusing.
		if strings.Contains(cfg.HTTPAddress.String(), "0.0.0.0") {
			listenAddrStr = strings.ReplaceAll(listenAddrStr, "[::]", "0.0.0.0")
		}
		return listenAddrStr
	}

	if cfg.HTTPAddress.String() != "" {
		httpServers.HTTPListener, err = net.Listen("tcp", cfg.HTTPAddress.String())
		if err != nil {
			return nil, err
		}

		// We want to print out the address the user supplied, not the
		// loopback device.
		_, _ = fmt.Fprintf(inv.Stdout, "Started HTTP listener at %s\n", (&url.URL{Scheme: "http", Host: addrString(httpServers.HTTPListener)}).String())

		// Set the http URL we want to use when connecting to ourselves.
		tcpAddr, tcpAddrValid := httpServers.HTTPListener.Addr().(*net.TCPAddr)
		if !tcpAddrValid {
			return nil, xerrors.Errorf("invalid TCP address type %T", httpServers.HTTPListener.Addr())
		}
		if tcpAddr.IP.IsUnspecified() {
			tcpAddr.IP = net.IPv4(127, 0, 0, 1)
		}
		httpServers.HTTPUrl = &url.URL{
			Scheme: "http",
			Host:   tcpAddr.String(),
		}
	}

	if cfg.TLS.Enable {
		if cfg.TLS.Address.String() == "" {
			return nil, xerrors.New("tls address must be set if tls is enabled")
		}

		redirectHTTPToHTTPSDeprecation(ctx, logger, inv, cfg)

		tlsConfig, err := configureServerTLS(
			ctx,
			logger,
			cfg.TLS.MinVersion.String(),
			cfg.TLS.ClientAuth.String(),
			cfg.TLS.CertFiles,
			cfg.TLS.KeyFiles,
			cfg.TLS.ClientCAFile.String(),
			cfg.TLS.SupportedCiphers.Value(),
			cfg.TLS.AllowInsecureCiphers.Value(),
		)
		if err != nil {
			return nil, xerrors.Errorf("configure tls: %w", err)
		}
		httpsListenerInner, err := net.Listen("tcp", cfg.TLS.Address.String())
		if err != nil {
			return nil, err
		}

		httpServers.TLSConfig = tlsConfig
		httpServers.TLSListener = tls.NewListener(httpsListenerInner, tlsConfig)

		// We want to print out the address the user supplied, not the
		// loopback device.
		_, _ = fmt.Fprintf(inv.Stdout, "Started TLS/HTTPS listener at %s\n", (&url.URL{Scheme: "https", Host: addrString(httpServers.TLSListener)}).String())

		// Set the https URL we want to use when connecting to
		// ourselves.
		tcpAddr, tcpAddrValid := httpServers.TLSListener.Addr().(*net.TCPAddr)
		if !tcpAddrValid {
			return nil, xerrors.Errorf("invalid TCP address type %T", httpServers.TLSListener.Addr())
		}
		if tcpAddr.IP.IsUnspecified() {
			tcpAddr.IP = net.IPv4(127, 0, 0, 1)
		}
		httpServers.TLSUrl = &url.URL{
			Scheme: "https",
			Host:   tcpAddr.String(),
		}
	}

	if httpServers.HTTPListener == nil && httpServers.TLSListener == nil {
		return nil, xerrors.New("must listen on at least one address")
	}

	return httpServers, nil
}

// redirectHTTPToHTTPSDeprecation handles deprecation of the --tls-redirect-http-to-https flag and
// "related" environment variables.
//
// --tls-redirect-http-to-https used to default to true.
// It made more sense to have the redirect be opt-in.
//
// Also, for a while we have been accepting the environment variable (but not the
// corresponding flag!) "CODER_TLS_REDIRECT_HTTP", and it appeared in a configuration
// example, so we keep accepting it to not break backward compat.
func redirectHTTPToHTTPSDeprecation(ctx context.Context, logger slog.Logger, inv *serpent.Invocation, cfg *codersdk.DeploymentValues) {
	truthy := func(s string) bool {
		b, err := strconv.ParseBool(s)
		if err != nil {
			return false
		}
		return b
	}
	if truthy(inv.Environ.Get("CODER_TLS_REDIRECT_HTTP")) ||
		truthy(inv.Environ.Get("CODER_TLS_REDIRECT_HTTP_TO_HTTPS")) ||
		inv.ParsedFlags().Changed("tls-redirect-http-to-https") {
		logger.Warn(ctx, "⚠️ --tls-redirect-http-to-https is deprecated, please use --redirect-to-access-url instead")
		cfg.RedirectToAccessURL = cfg.TLS.RedirectHTTP
	}
}

// ReadExternalAuthProvidersFromEnv is provided for compatibility purposes with
// the viper CLI.
func ReadExternalAuthProvidersFromEnv(environ []string) ([]codersdk.ExternalAuthConfig, error) {
	providers, err := parseExternalAuthProvidersFromEnv("CODER_EXTERNAL_AUTH_", environ)
	if err != nil {
		return nil, err
	}
	// Deprecated: To support legacy git auth!
	gitProviders, err := parseExternalAuthProvidersFromEnv("CODER_GITAUTH_", environ)
	if err != nil {
		return nil, err
	}
	return append(providers, gitProviders...), nil
}

// parseExternalAuthProvidersFromEnv consumes environment variables to parse
// external auth providers. A prefix is provided to support the legacy
// parsing of `GITAUTH` environment variables.
func parseExternalAuthProvidersFromEnv(prefix string, environ []string) ([]codersdk.ExternalAuthConfig, error) {
	// The index numbers must be in-order.
	sort.Strings(environ)

	var providers []codersdk.ExternalAuthConfig
	for _, v := range serpent.ParseEnviron(environ, prefix) {
		tokens := strings.SplitN(v.Name, "_", 2)
		if len(tokens) != 2 {
			return nil, xerrors.Errorf("invalid env var: %s", v.Name)
		}

		providerNum, err := strconv.Atoi(tokens[0])
		if err != nil {
			return nil, xerrors.Errorf("parse number: %s", v.Name)
		}

		var provider codersdk.ExternalAuthConfig
		switch {
		case len(providers) < providerNum:
			return nil, xerrors.Errorf(
				"provider num %v skipped: %s",
				len(providers),
				v.Name,
			)
		case len(providers) == providerNum:
			// At the next next provider.
			providers = append(providers, provider)
		case len(providers) == providerNum+1:
			// At the current provider.
			provider = providers[providerNum]
		}

		key := tokens[1]
		switch key {
		case "ID":
			provider.ID = v.Value
		case "TYPE":
			provider.Type = v.Value
		case "CLIENT_ID":
			provider.ClientID = v.Value
		case "CLIENT_SECRET":
			provider.ClientSecret = v.Value
		case "AUTH_URL":
			provider.AuthURL = v.Value
		case "TOKEN_URL":
			provider.TokenURL = v.Value
		case "VALIDATE_URL":
			provider.ValidateURL = v.Value
		case "REGEX":
			provider.Regex = v.Value
		case "DEVICE_FLOW":
			b, err := strconv.ParseBool(v.Value)
			if err != nil {
				return nil, xerrors.Errorf("parse bool: %s", v.Value)
			}
			provider.DeviceFlow = b
		case "DEVICE_CODE_URL":
			provider.DeviceCodeURL = v.Value
		case "NO_REFRESH":
			b, err := strconv.ParseBool(v.Value)
			if err != nil {
				return nil, xerrors.Errorf("parse bool: %s", v.Value)
			}
			provider.NoRefresh = b
		case "SCOPES":
			provider.Scopes = strings.Split(v.Value, " ")
		case "EXTRA_TOKEN_KEYS":
			provider.ExtraTokenKeys = strings.Split(v.Value, " ")
		case "APP_INSTALL_URL":
			provider.AppInstallURL = v.Value
		case "APP_INSTALLATIONS_URL":
			provider.AppInstallationsURL = v.Value
		case "DISPLAY_NAME":
			provider.DisplayName = v.Value
		case "DISPLAY_ICON":
			provider.DisplayIcon = v.Value
		}
		providers[providerNum] = provider
	}
	return providers, nil
}

// If the user provides a postgres URL with a password that contains special
// characters, the URL will be invalid. We need to escape the password so that
// the URL parse doesn't fail at the DB connector level.
func escapePostgresURLUserInfo(v string) (string, error) {
	_, err := url.Parse(v)
	// I wish I could use errors.Is here, but this error is not declared as a
	// variable in net/url. :(
	if err != nil {
		if strings.Contains(err.Error(), "net/url: invalid userinfo") {
			// If the URL is invalid, we assume it is because the password contains
			// special characters that need to be escaped.

			// get everything before first @
			parts := strings.SplitN(v, "@", 2)
			if len(parts) != 2 {
				return "", xerrors.Errorf("invalid postgres url with userinfo: %s", v)
			}
			start := parts[0]
			// get password, which is the last item in start when split by :
			startParts := strings.Split(start, ":")
			password := startParts[len(startParts)-1]
			// escape password, and replace the last item in the startParts slice
			// with the escaped password.
			//
			// url.PathEscape is used here because url.QueryEscape
			// will not escape spaces correctly.
			newPassword := url.PathEscape(password)
			startParts[len(startParts)-1] = newPassword
			start = strings.Join(startParts, ":")
			return start + "@" + parts[1], nil
		}

		return "", xerrors.Errorf("parse postgres url: %w", err)
	}

	return v, nil
}

func signalNotifyContext(ctx context.Context, inv *serpent.Invocation, sig ...os.Signal) (context.Context, context.CancelFunc) {
	// On Windows, some of our signal functions lack support.
	// If we pass in no signals, we should just return the context as-is.
	if len(sig) == 0 {
		return context.WithCancel(ctx)
	}
	return inv.SignalNotifyContext(ctx, sig...)
}
