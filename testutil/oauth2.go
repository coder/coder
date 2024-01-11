package testutil

import (
	"context"
	"net/http"
	"net/url"
	"time"

	"golang.org/x/oauth2"

	"github.com/coder/coder/v2/coderd/promoauth"
)

type OAuth2Config struct {
	Token           *oauth2.Token
	TokenSourceFunc OAuth2TokenSource
}

func (*OAuth2Config) Do(_ context.Context, _ promoauth.Oauth2Source, req *http.Request) (*http.Response, error) {
	return http.DefaultClient.Do(req)
}

func (*OAuth2Config) AuthCodeURL(state string, _ ...oauth2.AuthCodeOption) string {
	return "/?state=" + url.QueryEscape(state)
}

func (c *OAuth2Config) Exchange(_ context.Context, _ string, _ ...oauth2.AuthCodeOption) (*oauth2.Token, error) {
	if c.Token == nil {
		return &oauth2.Token{
			AccessToken:  "access_token",
			RefreshToken: "refresh_token",
			Expiry:       time.Now().Add(time.Hour),
		}, nil
	}
	return c.Token, nil
}

func (c *OAuth2Config) TokenSource(_ context.Context, _ *oauth2.Token) oauth2.TokenSource {
	if c.TokenSourceFunc == nil {
		return OAuth2TokenSource(func() (*oauth2.Token, error) {
			if c.Token == nil {
				return &oauth2.Token{
					AccessToken:  "access_token",
					RefreshToken: "refresh_token",
					Expiry:       time.Now().Add(time.Hour),
				}, nil
			}
			return c.Token, nil
		})
	}
	return c.TokenSourceFunc
}

type OAuth2TokenSource func() (*oauth2.Token, error)

func (o OAuth2TokenSource) Token() (*oauth2.Token, error) {
	return o()
}
