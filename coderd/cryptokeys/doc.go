// Package cryptokeys provides an abstraction for fetching internally used cryptographic keys mainly for JWT signing and verification.
package cryptokeys

//go:generate mockgen -destination mock_keycache.go -package cryptokeys . Keycache
