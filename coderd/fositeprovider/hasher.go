package fositeprovider

import (
	"context"
	"strings"

	"github.com/ory/fosite"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/userpassword"
)

var _ fosite.Hasher = (*clientSecretHasher)(nil)

type clientSecretHasher struct {
}

func (c clientSecretHasher) Compare(ctx context.Context, hashedSecret, data []byte) error {
	// TODO: Maybe there is a better place to do this parsing?
	parsed, err := parseFormattedSecret(string(data))
	if err != nil {
		return xerrors.Errorf("parse formatted secret: %w", err)
	}

	equal, err := userpassword.Compare(string(hashedSecret), parsed.secret)
	if err != nil {
		return xerrors.Errorf("compare hashed secret: %w", err)
	}

	if !equal {
		return xerrors.New("hashes do not match")
	}
	return nil
}

func (c clientSecretHasher) Hash(ctx context.Context, data []byte) ([]byte, error) {
	hashed, err := userpassword.Hash(string(data))
	if err != nil {
		return nil, xerrors.Errorf("hash secret: %w", err)
	}
	return []byte(hashed), nil
}

type parsedSecret struct {
	prefix string
	secret string
}

// parseFormattedSecret parses a formatted secret like "coder_prefix_secret"
func parseFormattedSecret(secret string) (parsedSecret, error) {
	parts := strings.Split(secret, "_")
	if len(parts) != 3 {
		return parsedSecret{}, xerrors.Errorf("incorrect number of parts: %d", len(parts))
	}
	if parts[0] != "coder" {
		return parsedSecret{}, xerrors.Errorf("incorrect scheme: %s", parts[0])
	}
	return parsedSecret{
		prefix: parts[1],
		secret: parts[2],
	}, nil
}
