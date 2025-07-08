package oauth2provider

import (
	"fmt"

	"github.com/coder/coder/v2/coderd/userpassword"
	"github.com/coder/coder/v2/cryptorand"
)

type AppSecret struct {
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
func GenerateSecret() (AppSecret, error) {
	// 40 characters matches the length of GitHub's client secrets.
	secret, err := cryptorand.String(40)
	if err != nil {
		return AppSecret{}, err
	}

	// This ID is prefixed to the secret so it can be used to look up the secret
	// when the user provides it, since we cannot just re-hash it to match as we
	// will not have the salt.
	prefix, err := cryptorand.String(10)
	if err != nil {
		return AppSecret{}, err
	}

	hashed, err := userpassword.Hash(secret)
	if err != nil {
		return AppSecret{}, err
	}

	return AppSecret{
		Formatted: fmt.Sprintf("coder_%s_%s", prefix, secret),
		Prefix:    prefix,
		Hashed:    hashed,
	}, nil
}
