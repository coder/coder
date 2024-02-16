package identityprovider

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/cryptorand"
)

type OAuth2ProviderAppSecret struct {
	// Formatted contains the secret. This value is owned by the client, not the
	// server.  It is formatted to include the prefix.
	Formatted string
	// Prefix is the ID of this secret owned by the server. When a client uses a
	// secret, this is the matching string to do a lookup on the hashed value.  We
	// cannot use the hashed value directly because the server does not store the
	// salt.
	Prefix string
	// Hashed is the server stored hash(secret,salt,...). Used for verifying a
	// secret.
	Hashed string
}

// GenerateSecret generates a secret to be used as a client secret, refresh
// token, or authorization code.
func GenerateSecret() (OAuth2ProviderAppSecret, error) {
	// 40 characters matches the length of GitHub's client secrets.
	secret, err := cryptorand.String(40)
	if err != nil {
		return OAuth2ProviderAppSecret{}, err
	}

	// This ID is prefixed to the secret so it can be used to look up the secret
	// when the user provides it, since we cannot just re-hash it to match as we
	// will not have the salt.
	prefix, err := cryptorand.String(10)
	if err != nil {
		return OAuth2ProviderAppSecret{}, err
	}

	hashed, err := userpassword.Hash(secret)
	if err != nil {
		return OAuth2ProviderAppSecret{}, err
	}

	return OAuth2ProviderAppSecret{
		Formatted: fmt.Sprintf("coder_%s_%s", prefix, secret),
		Prefix:    prefix,
		Hashed:    hashed,
	}, nil
}

type parsedSecret struct {
	prefix string
	secret string
}

// parseSecret extracts the ID and original secret from a secret.
func parseSecret(secret string) (parsedSecret, error) {
	parts := strings.Split(secret, "_")
	if len(parts) != 3 {
		return parsedSecret{}, xerrors.Errorf("incorrect number of parts: %d", len(parts))
	}
	if parts[0] != "coder" {
		return parsedSecret{}, xerrors.Errorf("incorrect scheme: %s", parts[0])
	}
	if len(parts[1]) == 0 {
		return parsedSecret{}, xerrors.Errorf("prefix is invalid")
	}
	if len(parts[2]) == 0 {
		return parsedSecret{}, xerrors.Errorf("invalid")
	}
	return parsedSecret{parts[1], parts[2]}, nil
}
