package scim

import (
	"fmt"
	"net/http"

	"github.com/elimity-com/scim"
	"github.com/elimity-com/scim/optional"
	"github.com/google/uuid"

	"github.com/coder/coder/v2/coderd/database"
)

var _ scim.ResourceHandler = (*ResourceUser)(nil)

type ResourceUser struct {
	store database.Store
}

func (ru *ResourceUser) Create(r *http.Request, attributes scim.ResourceAttributes) (scim.Resource, error) {
	//TODO implement me
	panic("implement me")
}

func (ru *ResourceUser) Get(r *http.Request, idStr string) (scim.Resource, error) {
	ctx := r.Context()
	id, err := uuid.Parse(idStr)
	if err != nil {
		return scim.Resource{}, fmt.Errorf("invalid user ID %q: %w", idStr, err)
	}

	usr, err := ru.store.GetUserByID(ctx, id)
	if err != nil {
		return scim.Resource{}, err
	}

	return userResource(usr), nil
}

func (ru *ResourceUser) GetAll(r *http.Request, params scim.ListRequestParams) (scim.Page, error) {
	//TODO implement me
	panic("implement me")
}

func (ru *ResourceUser) Replace(r *http.Request, id string, attributes scim.ResourceAttributes) (scim.Resource, error) {
	//TODO implement me
	panic("implement me")
}

func (ru *ResourceUser) Delete(r *http.Request, id string) error {
	//TODO implement me
	panic("implement me")
}

func (ru *ResourceUser) Patch(r *http.Request, id string, operations []scim.PatchOperation) (scim.Resource, error) {
	//TODO implement me
	panic("implement me")
}

func userResource(u database.User) scim.Resource {
	return scim.Resource{
		ID:         u.ID.String(),
		ExternalID: optional.String{},
		Attributes: scim.ResourceAttributes{
			"name": map[string]interface{}{
				"givenName": u.Username,
			},
			"emails": []map[string]interface{}{
				{
					"primary": true,
					"value":   u.Email,
				},
			},
			"active": u.Status == database.UserStatusActive,
			// TODO: Groups
		},
		Meta: scim.Meta{
			Created:      &u.CreatedAt,
			LastModified: &u.UpdatedAt,
		},
	}
}
