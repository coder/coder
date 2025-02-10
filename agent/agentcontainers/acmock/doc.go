// Package acmock contains a mock implementation of agentcontainers.Lister for use in tests.
package acmock

//go:generate mockgen -destination ./acmock.go -package acmock .. Lister
