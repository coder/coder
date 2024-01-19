package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/require"

	"cdr.dev/slog"
	"cdr.dev/slog/sloggers/sloghuman"
	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
	"github.com/coder/coder/v2/codersdk"
)

// Flags
var (
	expiry       = flag.Duration("expiry", time.Minute*5, "Token expiry")
	clientID     = flag.String("client-id", "static-client-id", "Client ID, set empty to be random")
	clientSecret = flag.String("client-sec", "static-client-secret", "Client Secret, set empty to be random")
	deviceFlow   = flag.Bool("device-flow", false, "Enable device flow")
	// By default, no regex means it will never match anything. So at least default to matching something.
	extRegex = flag.String("ext-regex", `^(https?://)?example\.com(/.*)?$`, "External auth regex")
)

func main() {
	testing.Init()
	_ = flag.Set("test.timeout", "0")

	flag.Parse()

	// This is just a way to run tests outside go test
	testing.Main(func(pat, str string) (bool, error) {
		return true, nil
	}, []testing.InternalTest{
		{
			Name: "Run Fake IDP",
			F:    RunIDP(),
		},
	}, nil, nil)
}

type withClientSecret struct {
	// We never unmarshal this in prod, but we need this field for testing.
	ClientSecret string `json:"client_secret"`
	codersdk.ExternalAuthConfig
}

// RunIDP needs the testing.T because our oidctest package requires the
// testing.T.
func RunIDP() func(t *testing.T) {
	return func(t *testing.T) {
		idp := oidctest.NewFakeIDP(t,
			oidctest.WithServing(),
			oidctest.WithStaticUserInfo(jwt.MapClaims{}),
			oidctest.WithDefaultIDClaims(jwt.MapClaims{}),
			oidctest.WithDefaultExpire(*expiry),
			oidctest.WithStaticCredentials(*clientID, *clientSecret),
			oidctest.WithIssuer("http://localhost:4500"),
			oidctest.WithLogger(slog.Make(sloghuman.Sink(os.Stderr))),
		)
		id, sec := idp.AppCredentials()
		prov := idp.WellknownConfig()
		const appID = "fake"
		coderCfg := idp.ExternalAuthConfig(t, appID, &oidctest.ExternalAuthConfigOptions{
			UseDeviceAuth: *deviceFlow,
		})

		log.Println("IDP Issuer URL", idp.IssuerURL())
		log.Println("Coderd Flags")

		deviceCodeURL := ""
		if coderCfg.DeviceAuth != nil {
			deviceCodeURL = coderCfg.DeviceAuth.CodeURL
		}

		cfg := withClientSecret{
			ClientSecret: sec,
			ExternalAuthConfig: codersdk.ExternalAuthConfig{
				Type:                appID,
				ClientID:            id,
				ClientSecret:        sec,
				ID:                  appID,
				AuthURL:             prov.AuthURL,
				TokenURL:            prov.TokenURL,
				ValidateURL:         prov.ExternalAuthURL,
				AppInstallURL:       coderCfg.AppInstallURL,
				AppInstallationsURL: coderCfg.AppInstallationsURL,
				NoRefresh:           false,
				Scopes:              []string{"openid", "email", "profile"},
				ExtraTokenKeys:      coderCfg.ExtraTokenKeys,
				DeviceFlow:          *deviceFlow,
				DeviceCodeURL:       deviceCodeURL,
				Regex:               *extRegex,
				DisplayName:         coderCfg.DisplayName,
				DisplayIcon:         coderCfg.DisplayIcon,
			},
		}

		data, err := json.Marshal([]withClientSecret{cfg})
		require.NoError(t, err)
		log.Printf(`--external-auth-providers='%s'`, string(data))

		log.Println("Press Ctrl+C to exit")
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)

		// Block until ctl+c
		<-c
		log.Println("Closing")
	}
}
