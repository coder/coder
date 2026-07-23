// Package acmock contains a mock implementation of agentcontainers.Lister for use in tests.
package acmock

//go:generate go tool mockgen -destination ./acmock.go -package acmock .. ContainerCLI,DevcontainerCLI,SubAgentClient
