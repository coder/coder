package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"
	"os/signal"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
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
	extRegex        = flag.String("ext-regex", `^(https?://)?example\.com(/.*)?$`, "External auth regex")
	tooManyRequests = flag.String("429", "", "Simulate too many requests for a given endpoint.")
)

func main() {
	testing.Init()
	_ = flag.Set("test.timeout", "0")

	flag.Parse()

	// This is just a way to run tests outside go test
	testing.Main(func(_, _ string) (bool, error) {
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
	tooManyRequestParams := oidctest.With429Arguments{}
	if *tooManyRequests != "" {
		for _, v := range strings.Split(*tooManyRequests, ",") {
			v = strings.ToLower(strings.TrimSpace(v))
			switch v {
			case "all":
				tooManyRequestParams.AllPaths = true
			case "auth":
				tooManyRequestParams.AuthorizePath = true
			case "token":
				tooManyRequestParams.TokenPath = true
			case "keys":
				tooManyRequestParams.KeysPath = true
			case "userinfo":
				tooManyRequestParams.UserInfoPath = true
			case "device":
				tooManyRequestParams.DeviceAuth = true
			case "device-verify":
				tooManyRequestParams.DeviceVerify = true
			default:
				log.Printf("Unknown too-many-requests value: %s\nView the `testidp/main.go` for valid values.", v)
			}
		}
	}

	return func(t *testing.T) {
		idp := oidctest.NewFakeIDP(t,
			oidctest.WithServing(),
			oidctest.WithStaticUserInfo(jwt.MapClaims{
				// This is a static set of auth fields. Might be beneficial to make flags
				// to allow different values here. This is only required for using the
				// testIDP as primary auth. External auth does not ever fetch these fields.
				"sub":                uuid.MustParse("26c6a19c-b9b8-493b-a991-88a4c3310314"),
				"email":              "oidc_member@coder.com",
				"preferred_username": "oidc_member",
				"email_verified":     true,
				"groups":             []string{"testidp", "qa", "engineering"},
				"roles":              []string{"testidp", "admin", "higher_power"},
			}),
			oidctest.WithDefaultIDClaims(jwt.MapClaims{}),
			oidctest.WithDefaultExpire(*expiry),
			oidctest.WithStaticCredentials(*clientID, *clientSecret),
			oidctest.WithIssuer("http://localhost:4500"),
			oidctest.WithLogger(slog.Make(sloghuman.Sink(os.Stderr))),
			oidctest.With429(tooManyRequestParams),
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
		log.Println("As primary OIDC auth")
		log.Printf(`--oidc-issuer-url=%s --oidc-client-id=%s --oidc-client-secret=%s`, idp.IssuerURL().String(), *clientID, *clientSecret)

		log.Println("Press Ctrl+C to exit")
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)

		// Block until ctl+c
		<-c
		log.Println("Closing")
	}
}
