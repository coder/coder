package aibridge

import "github.com/coder/coder/v2/aibridge/keypool"

// CollectKeyPools returns the providers' non-nil key pools in provider order.
func CollectKeyPools(providers []Provider) []*keypool.Pool {
	pools := make([]*keypool.Pool, 0, len(providers))
	for _, prov := range providers {
		if pool := prov.KeyPool(); pool != nil {
			pools = append(pools, pool)
		}
	}
	return pools
}
