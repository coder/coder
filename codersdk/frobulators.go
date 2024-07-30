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
	ModelNumber string    `json:"model_number"`
}

type InsertFrobulatorRequest struct {
	ModelNumber string `json:"model_number"`
}

func (c *Client) CreateFrobulator(ctx context.Context, userID uuid.UUID, modelNumber string) (uuid.UUID, error) {
	req := InsertFrobulatorRequest{
		ModelNumber: modelNumber,
	}

	res, err := c.Request(ctx, http.MethodPost, fmt.Sprintf("/api/v2/frobulators/%s", userID.String()), req)
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

func (c *Client) GetUserFrobulators(ctx context.Context, userID uuid.UUID) ([]Frobulator, error) {
	res, err := c.Request(ctx, http.MethodGet, fmt.Sprintf("/api/v2/frobulators/%s", userID.String()), nil)
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

func (c *Client) GetAllFrobulators(ctx context.Context) ([]Frobulator, error) {
	res, err := c.Request(ctx, http.MethodGet, "/api/v2/frobulators", nil)
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
