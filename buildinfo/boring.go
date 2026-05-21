//go:build boringcrypto

package buildinfo

import "crypto/boring"

var boringcrypto = boring.Enabled()
