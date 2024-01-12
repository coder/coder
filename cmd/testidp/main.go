package main

import (
	"flag"
	"log"
	"os"
	"os/signal"
	"testing"

	"github.com/golang-jwt/jwt/v4"

	"github.com/coder/coder/v2/coderd/coderdtest/oidctest"
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

// RunIDP needs the testing.T because our oidctest package requires the
// testing.T.
func RunIDP() func(t *testing.T) {
	return func(t *testing.T) {
		idp := oidctest.NewFakeIDP(t,
			oidctest.WithServing(),
			oidctest.WithStaticUserInfo(jwt.MapClaims{}),
			oidctest.WithDefaultIDClaims(jwt.MapClaims{}),
		)
		id, sec := idp.AppCredentials()
		prov := idp.WellknownConfig()

		log.Println("IDP Issuer URL", idp.IssuerURL())
		log.Println("Coderd Flags")
		log.Printf(`--external-auth-providers='[{"type":"fake","client_id":"%s","client_secret":"%s","auth_url":"%s","token_url":"%s","validate_url":"%s","scopes":["openid","email","profile"]}]'`,
			id, sec, prov.AuthURL, prov.TokenURL, prov.UserInfoURL,
		)

		log.Println("Press Ctrl+C to exit")
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt)

		// Block until ctl+c
		<-c
		log.Println("Closing")
	}
}
