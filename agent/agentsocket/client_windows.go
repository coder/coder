//go:build windows

package agentsocket

import (
	"context"

	"golang.org/x/xerrors"
)

// Client provides a client for communicating with the workspace agentsocket API.
type Client struct{}

// NewClient returns an error indicating that agentsocket is not supported on Windows.
func NewClient(ctx context.Context, opts ...Option) (*Client, error) {
	return nil, xerrors.New("agentsocket is not supported on Windows")
}

// Close closes the socket connection.
func (c *Client) Close() error {
	return nil
}

// Ping sends a ping request to the agent.
func (c *Client) Ping(ctx context.Context) error {
	return xerrors.New("agentsocket is not supported on Windows")
}

// SyncStart starts a unit in the dependency graph.
func (c *Client) SyncStart(ctx context.Context, unitName string) error {
	return xerrors.New("agentsocket is not supported on Windows")
}

// SyncWant declares a dependency between units.
func (c *Client) SyncWant(ctx context.Context, unitName, dependsOn string) error {
	return xerrors.New("agentsocket is not supported on Windows")
}

// SyncComplete marks a unit as complete in the dependency graph.
func (c *Client) SyncComplete(ctx context.Context, unitName string) error {
	return xerrors.New("agentsocket is not supported on Windows")
}

// SyncReady requests whether a unit is ready to be started. That is, all dependencies are satisfied.
func (c *Client) SyncReady(ctx context.Context, unitName string) (bool, error) {
	return false, xerrors.New("agentsocket is not supported on Windows")
}

// SyncStatus gets the status of a unit and its dependencies.
func (c *Client) SyncStatus(ctx context.Context, unitName string) (SyncStatusResponse, error) {
	return SyncStatusResponse{}, xerrors.New("agentsocket is not supported on Windows")
}
