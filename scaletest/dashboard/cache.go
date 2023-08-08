package dashboard

import (
	"context"
	"math/rand"
	"sync"

	"github.com/coder/coder/codersdk"
)

type cache struct {
	sync.RWMutex
	workspaces []codersdk.Workspace
	templates  []codersdk.Template
	users      []codersdk.User
}

func (c *cache) fill(ctx context.Context, client *codersdk.Client) error {
	c.Lock()
	defer c.Unlock()
	me, err := client.User(ctx, codersdk.Me)
	if err != nil {
		return err
	}
	ws, err := client.Workspaces(ctx, codersdk.WorkspaceFilter{})
	if err != nil {
		return err
	}
	c.workspaces = ws.Workspaces
	tpl, err := client.TemplatesByOrganization(ctx, me.OrganizationIDs[0])
	if err != nil {
		return err
	}
	c.templates = tpl
	users, err := client.Users(ctx, codersdk.UsersRequest{})
	if err != nil {
		return err
	}
	c.users = users.Users
	return nil
}

func (c *cache) setWorkspaces(ws []codersdk.Workspace) {
	c.Lock()
	c.workspaces = ws
	c.Unlock()
}

func (c *cache) setTemplates(t []codersdk.Template) {
	c.Lock()
	c.templates = t
	c.Unlock()
}

func (c *cache) randWorkspace() codersdk.Workspace {
	c.RLock()
	defer c.RUnlock()
	if len(c.workspaces) == 0 {
		return codersdk.Workspace{}
	}
	return pick(c.workspaces)
}

func (c *cache) randTemplate() codersdk.Template {
	c.RLock()
	defer c.RUnlock()
	if len(c.templates) == 0 {
		return codersdk.Template{}
	}
	return pick(c.templates)
}

func (c *cache) setUsers(u []codersdk.User) {
	c.Lock()
	c.users = u
	c.Unlock()
}

func (c *cache) randUser() codersdk.User {
	c.RLock()
	defer c.RUnlock()
	if len(c.users) == 0 {
		return codersdk.User{}
	}
	return pick(c.users)
}

// pick chooses a random element from a slice.
// If the slice is empty, it returns the zero value of the type.
func pick[T any](s []T) T {
	if len(s) == 0 {
		var zero T
		return zero
	}
	// nolint:gosec
	return s[rand.Intn(len(s))]
}
