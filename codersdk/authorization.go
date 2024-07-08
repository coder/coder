package codersdk

import (
	"context"
	"encoding/json"
	"net/http"
)

type AuthorizationResponse map[string]bool

// AuthorizationRequest is a structure instead of a map because
// go-playground/validate can only validate structs. If you attempt to pass
// a map into `httpapi.Read`, you will get an invalid type error.
type AuthorizationRequest struct {
	// Checks is a map keyed with an arbitrary string to a permission check.
	// The key can be any string that is helpful to the caller, and allows
	// multiple permission checks to be run in a single request.
	// The key ensures that each permission check has the same key in the
	// response.
	Checks map[string]AuthorizationCheck `json:"checks"`
}

// AuthorizationCheck is used to check if the currently authenticated user (or the specified user) can do a given action to a given set of objects.
//
// @Description AuthorizationCheck is used to check if the currently authenticated user (or the specified user) can do a given action to a given set of objects.
type AuthorizationCheck struct {
	// Object can represent a "set" of objects, such as: all workspaces in an organization, all workspaces owned by me, and all workspaces across the entire product.
	// When defining an object, use the most specific language when possible to
	// produce the smallest set. Meaning to set as many fields on 'Object' as
	// you can. Example, if you want to check if you can update all workspaces
	// owned by 'me', try to also add an 'OrganizationID' to the settings.
	// Omitting the 'OrganizationID' could produce the incorrect value, as
	// workspaces have both `user` and `organization` owners.
	Object AuthorizationObject `json:"object"`
	Action RBACAction          `json:"action" enums:"create,read,update,delete"`
}

// AuthorizationObject can represent a "set" of objects, such as: all workspaces in an organization, all workspaces owned by me,
// all workspaces across the entire product.
//
// @Description AuthorizationObject can represent a "set" of objects, such as: all workspaces in an organization, all workspaces owned by me,
// @Description all workspaces across the entire product.
type AuthorizationObject struct {
	// ResourceType is the name of the resource.
	// `./coderd/rbac/object.go` has the list of valid resource types.
	ResourceType RBACResource `json:"resource_type"`
	// OwnerID (optional) adds the set constraint to all resources owned by a given user.
	OwnerID string `json:"owner_id,omitempty"`
	// OrganizationID (optional) adds the set constraint to all resources owned by a given organization.
	OrganizationID string `json:"organization_id,omitempty"`
	// ResourceID (optional) reduces the set to a singular resource. This assigns
	// a resource ID to the resource type, eg: a single workspace.
	// The rbac library will not fetch the resource from the database, so if you
	// are using this option, you should also set the owner ID and organization ID
	// if possible. Be as specific as possible using all the fields relevant.
	ResourceID string `json:"resource_id,omitempty"`
}

// AuthCheck allows the authenticated user to check if they have the given permissions
// to a set of resources.
func (c *Client) AuthCheck(ctx context.Context, req AuthorizationRequest) (AuthorizationResponse, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/authcheck", req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AuthorizationResponse{}, ReadBodyAsError(res)
	}
	var resp AuthorizationResponse
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
