package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/google/uuid"
)

type Frobulator struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	UserID      uuid.UUID `json:"user_id" format:"uuid"`
	OrgID       uuid.UUID `json:"org_id" format:"uuid"`
	ModelNumber string    `json:"model_number"`
}

type InsertFrobulatorRequest struct {
	ModelNumber string `json:"model_number"`
}

func (c *Client) CreateFrobulator(ctx context.Context, userID, orgID uuid.UUID, modelNumber string) (uuid.UUID, error) {
	req := InsertFrobulatorRequest{
		ModelNumber: modelNumber,
	}

	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/organizations/%s/frobulators/%s", orgID.String(), userID.String()), req)
	if err != nil {
		return uuid.Nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return uuid.Nil, ReadBodyAsError(res)
	}

	var newID uuid.UUID
	return newID, json.NewDecoder(res.Body).Decode(&newID)
}

func (c *Client) GetFrobulators(ctx context.Context, userID, orgID uuid.UUID) ([]Frobulator, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/organizations/%s/frobulators/%s", orgID.String(), userID.String()), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}

	var frobulators []Frobulator
	return frobulators, json.NewDecoder(res.Body).Decode(&frobulators)
}

func (c *Client) DeleteFrobulator(ctx context.Context, id, userID, orgID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, fmt.Sprintf("/api/v2/organizations/%s/frobulators/%s/%s", orgID.String(), userID.String(), id.String()), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return ReadBodyAsError(res)
	}

	return nil
}
