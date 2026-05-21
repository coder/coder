package oauth2provider

import (
	"fmt"
	"strings"

	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/apikey"
	"github.com/coder/coder/v2/cryptorand"
)

const (
	// SecretIdentifier is the prefix added to all generated secrets.
	SecretIdentifier = "coder"
)

// Constants for OAuth2 secret generation
const (
	secretLength        = 40 // Length of the actual secret part
	displaySecretLength = 6  // Length of visible part in UI (last 6 characters)
)

type HashedAppSecret struct {
	AppSecret
	// Hashed is the server stored hash(secret,salt,...). Used for verifying a
	// secret.
	Hashed []byte
}

type AppSecret struct {
	// Formatted contains the secret. This value is owned by the client, not the
	// server.  It is formatted to include the prefix.
	Formatted string
	// Secret is the raw secret value. This value should only be known to the client.
	Secret string
	// Prefix is the ID of this secret owned by the server. When a client uses a
	// secret, this is the matching string to do a lookup on the hashed value.  We
	// cannot use the hashed value directly because the server does not store the
	// salt.
	Prefix string
}

// ParseFormattedSecret parses a formatted secret like "coder_<prefix>_<secret"
func ParseFormattedSecret(formatted string) (AppSecret, error) {
	parts := strings.Split(formatted, "_")
	if len(parts) != 3 {
		return AppSecret{}, xerrors.Errorf("incorrect number of parts: %d", len(parts))
	}
	if parts[0] != SecretIdentifier {
		return AppSecret{}, xerrors.Errorf("incorrect scheme: %s", parts[0])
	}
	return AppSecret{
		Formatted: formatted,
		Prefix:    parts[1],
		Secret:    parts[2],
	}, nil
}

// GenerateSecret generates a secret to be used as a client secret, refresh
// token, or authorization code.
func GenerateSecret() (HashedAppSecret, error) {
	// 40 characters matches the length of GitHub's client secrets.
	secret, hashedSecret, err := apikey.GenerateSecret(40)
	if err != nil {
		return HashedAppSecret{}, err
	}

	// This ID is prefixed to the secret so it can be used to look up the secret
	// when the user provides it, since we cannot just re-hash it to match as we
	// will not have the salt.
	prefix, err := cryptorand.String(10)
	if err != nil {
		return HashedAppSecret{}, err
	}

	return HashedAppSecret{
		AppSecret: AppSecret{
			Formatted: fmt.Sprintf("%s_%s_%s", SecretIdentifier, prefix, secret),
			Secret:    secret,
			Prefix:    prefix,
		},
		Hashed: hashedSecret,
	}, nil
}
