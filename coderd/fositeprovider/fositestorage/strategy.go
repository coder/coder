// Copyright © 2025 Ory Corp
// SPDX-License-Identifier: Apache-2.0

package fositestorage

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/ory/fosite/handler/oauth2"
	enigma "github.com/ory/fosite/token/hmac"

	"github.com/ory/fosite"

	"github.com/coder/coder/v2/coderd/apikey"
)

var _ oauth2.CoreStrategy = (*TokenStrategy)(nil)

type TokenStrategy struct {
	original *oauth2.HMACSHAStrategyUnPrefixed
}

func (h *TokenStrategy) AccessTokenSignature(ctx context.Context, token string) string {
	return h.original.AccessTokenSignature(ctx, token)
}

func (h *TokenStrategy) RefreshTokenSignature(ctx context.Context, token string) string {
	return h.original.RefreshTokenSignature(ctx, token)
}

func (h *TokenStrategy) AuthorizeCodeSignature(ctx context.Context, token string) string {
	return h.original.AuthorizeCodeSignature(ctx, token)
}

func NewHMACSHAStrategy(
	enigma *enigma.HMACStrategy,
	config oauth2.LifespanConfigProvider,
) *TokenStrategy {
	return &TokenStrategy{
		original: oauth2.NewHMACSHAStrategyUnPrefixed(enigma, config),
	}
}

func (*TokenStrategy) splitPrefix(prefixedToken string) (prefix string, token string) {
	parts := strings.Split(prefixedToken, "-")
	if len(parts) != 2 {
		return "", prefixedToken
	}
	return parts[0], parts[1]
}

func (h *TokenStrategy) trimPrefix(prefixedToken string) string {
	_, token := h.splitPrefix(prefixedToken)
	return token
}

func (h *TokenStrategy) setPrefix(prefix, token string) string {
	return prefix + "-" + token
}

func (h *TokenStrategy) GenerateAccessToken(ctx context.Context, r fosite.Requester) (token string, signature string, err error) {
	id, sec, err := apikey.GenerateKey()
	if err != nil {
		return "", "", err
	}
	hashed := sha256.Sum256([]byte(sec))
	return sec, h.setPrefix(id, hex.EncodeToString(hashed[:])), nil
}

func (h *TokenStrategy) ValidateAccessToken(ctx context.Context, r fosite.Requester, token string) (err error) {
	return nil // We do not do any special validation here. If the token is in the database, it is valid.
}

func (h *TokenStrategy) GenerateRefreshToken(ctx context.Context, r fosite.Requester) (token string, signature string, err error) {
	token, sig, err := h.original.GenerateRefreshToken(ctx, r)
	return h.setPrefix(token, "rt"), sig, err // TODO: Prefix fix
}

func (h *TokenStrategy) ValidateRefreshToken(ctx context.Context, r fosite.Requester, token string) (err error) {
	return h.original.ValidateRefreshToken(ctx, r, h.trimPrefix(token))
}

func (h *TokenStrategy) GenerateAuthorizeCode(ctx context.Context, r fosite.Requester) (token string, signature string, err error) {
	token, sig, err := h.original.GenerateAuthorizeCode(ctx, r)
	return h.setPrefix(token, "ac"), sig, err // TODO: Prefix fix
}

func (h *TokenStrategy) ValidateAuthorizeCode(ctx context.Context, r fosite.Requester, token string) (err error) {
	return h.original.ValidateAuthorizeCode(ctx, r, h.trimPrefix(token))
}
