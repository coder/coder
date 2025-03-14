package wsproxy
import (
	"fmt"
	"errors"
	"context"
	"github.com/coder/coder/v2/coderd/cryptokeys"
	"github.com/coder/coder/v2/codersdk"
	"github.com/coder/coder/v2/enterprise/wsproxy/wsproxysdk"
)
var _ cryptokeys.Fetcher = &ProxyFetcher{}
type ProxyFetcher struct {
	Client *wsproxysdk.Client
}
func (p *ProxyFetcher) Fetch(ctx context.Context, feature codersdk.CryptoKeyFeature) ([]codersdk.CryptoKey, error) {
	keys, err := p.Client.CryptoKeys(ctx, feature)
	if err != nil {
		return nil, fmt.Errorf("crypto keys: %w", err)
	}
	return keys.CryptoKeys, nil
}
