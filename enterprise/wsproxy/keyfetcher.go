package wsproxy

import (
	"context"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
)

var _ cryptokeys.Fetcher = &ProxyFetcher{}

type ProxyFetcher struct {
	Client  *wsproxysdk.Client
	Feature codersdk.CryptoKeyFeature
}

func (p *ProxyFetcher) Fetch(ctx context.Context) ([]codersdk.CryptoKey, error) {
	keys, err := p.Client.CryptoKeys(ctx)
	if err != nil {
		return nil, xerrors.Errorf("crypto keys: %w", err)
	}
	return keys.CryptoKeys, nil
}
