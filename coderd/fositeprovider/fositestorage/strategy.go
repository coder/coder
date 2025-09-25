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

	"github.com/coder/coder/v2/cryptorand"
)

var _ oauth2.CoreStrategy = (*HashStrategy)(nil)

type HashStrategy struct {
}

func NewHashStrategy() *HashStrategy {
	return &HashStrategy{}
}

func (h HashStrategy) AccessTokenSignature(ctx context.Context, token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

func (h HashStrategy) GenerateAccessToken(ctx context.Context, requester fosite.Requester) (token string, signature string, err error) {
	secret, err := cryptorand.String(22)
	if err != nil {
		return "", "", err
	}

	return secret, h.AccessTokenSignature(ctx, secret), nil
}

func (h HashStrategy) ValidateAccessToken(ctx context.Context, requester fosite.Requester, token string) (err error) {
	// No validation is possible with hashing, so we just return nil here.
	return nil
}

func (h HashStrategy) RefreshTokenSignature(ctx context.Context, token string) string {
	return h.AccessTokenSignature(ctx, token) // Same as access token
}

func (h HashStrategy) GenerateRefreshToken(ctx context.Context, requester fosite.Requester) (token string, signature string, err error) {
	return h.GenerateAccessToken(ctx, requester) // Same as access token
}

func (h HashStrategy) ValidateRefreshToken(ctx context.Context, requester fosite.Requester, token string) (err error) {
	// No validation is possible with hashing, so we just return nil here.
	return nil
}

func (h HashStrategy) AuthorizeCodeSignature(ctx context.Context, token string) string {
	return h.AccessTokenSignature(ctx, token) // Same as access token
}

func (h HashStrategy) GenerateAuthorizeCode(ctx context.Context, requester fosite.Requester) (token string, signature string, err error) {
	return h.GenerateAccessToken(ctx, requester) // Same as access token
}

func (h HashStrategy) ValidateAuthorizeCode(ctx context.Context, requester fosite.Requester, token string) (err error) {
	// No validation is possible with hashing, so we just return nil here.
	return nil
}

// Using HMAC below

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

func NewTokenStrategy(
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
	token, sig, err := h.original.GenerateAccessToken(ctx, r)
	return h.setPrefix(token, "at"), sig, err // TODO: Prefix fix
}

func (h *TokenStrategy) ValidateAccessToken(ctx context.Context, r fosite.Requester, token string) (err error) {
	return h.original.ValidateAccessToken(ctx, r, token)
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
