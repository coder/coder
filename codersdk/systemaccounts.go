package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

type CreateSystemAccountRequest struct {
	Name string `json:"name" validate:"required"`
}

type SystemAccount struct {
	Name           string    `json:"name"`
	ID             uuid.UUID `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	OrganizationID uuid.UUID `json:"organization_id"`
	CreatedBy      string    `json:"created_by"`
}

// CreateSystemAccountResponse contains IDs for newly created systemaccount info.
type CreateSystemAccountResponse struct {
	Name           string    `json:"name"`
	ID             string    `json:"id"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
	OrganizationID string    `json:"organization_id"`
	CreatedBy      string    `json:"created_by"`
}

type UpdateSystemAccountRequest struct {
	Name string `json:"name" validate:"required,name"`
}

// CreateSystemAccount creates a new systemaccount.
func (c *Client) CreateSystemAccount(ctx context.Context, req CreateUserRequest) (SystemAccount, error) {
	res, err := c.Request(ctx, http.MethodPost, "/api/v2/systemaccounts", req)
	if err != nil {
		return SystemAccount{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return SystemAccount{}, ReadBodyAsError(res)
	}
	var systemaccount SystemAccount
	return systemaccount, json.NewDecoder(res.Body).Decode(&systemaccount)
}

// DeleteSystemAccount deletes a systemaccount.
func (c *Client) DeleteSystemAccount(ctx context.Context, id uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/systemaccount/%s", id), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}
	return nil
}

// UpdateSystemAccount enables callers to update profile information
func (c *Client) UpdateSystemAccount(ctx context.Context, systemaccount string, req UpdateSystemAccountRequest) (SystemAccount, error) {
	res, err := c.Request(ctx, http.MethodPut, fmt.Sprintf("/api/v2/systemaccount/%s/", systemaccount), req)
	if err != nil {
		return SystemAccount{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return SystemAccount{}, ReadBodyAsError(res)
	}
	var resp SystemAccount
	return resp, json.NewDecoder(res.Body).Decode(&resp)
}
