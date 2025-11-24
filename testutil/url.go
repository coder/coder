package testutil

import (
	"net/url"
	"testing"
)

func MustURL(t testing.TB, raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}
