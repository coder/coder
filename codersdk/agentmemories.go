package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type AgentMemoryEntryKind string

const (
	AgentMemoryEntryKindDirectory AgentMemoryEntryKind = "directory"
	AgentMemoryEntryKindMemory    AgentMemoryEntryKind = "memory"
)

// AgentMemoryEntry is a direct child in the virtual memory hierarchy.
type AgentMemoryEntry struct {
	Kind      AgentMemoryEntryKind `json:"kind"`
	Path      string               `json:"path"`
	ID        *uuid.UUID           `json:"id,omitempty" format:"uuid"`
	SizeBytes *int64               `json:"size_bytes,omitempty"`
	CreatedAt *time.Time           `json:"created_at,omitempty" format:"date-time"`
	UpdatedAt *time.Time           `json:"updated_at,omitempty" format:"date-time"`
}

type AgentMemoryChildrenResponse struct {
	Entries    []AgentMemoryEntry `json:"entries"`
	NextOffset *int32             `json:"next_offset,omitempty"`
}

type AgentMemory struct {
	ID        uuid.UUID `json:"id" format:"uuid"`
	Path      string    `json:"path"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at" format:"date-time"`
	UpdatedAt time.Time `json:"updated_at" format:"date-time"`
}

type UpdateAgentMemoryRequest struct {
	Content           string    `json:"content"`
	ExpectedUpdatedAt time.Time `json:"expected_updated_at" format:"date-time"`
}

func agentMemoriesPath(user string) string {
	return fmt.Sprintf("/api/experimental/users/%s/agent-memories", url.PathEscape(user))
}

func agentMemoryPath(user string, memoryID uuid.UUID) string {
	return fmt.Sprintf("%s/%s", agentMemoriesPath(user), memoryID)
}

func (c *ExperimentalClient) AgentMemoryChildren(ctx context.Context, user, directory string, offset int32) (AgentMemoryChildrenResponse, error) {
	values := url.Values{}
	values.Set("directory", directory)
	values.Set("offset", strconv.FormatInt(int64(offset), 10))
	res, err := c.Request(ctx, http.MethodGet, agentMemoriesPath(user)+"?"+values.Encode(), nil)
	if err != nil {
		return AgentMemoryChildrenResponse{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AgentMemoryChildrenResponse{}, ReadBodyAsError(res)
	}
	var response AgentMemoryChildrenResponse
	return response, json.NewDecoder(res.Body).Decode(&response)
}

func (c *ExperimentalClient) DefaultAgentMemory(ctx context.Context, user string) (AgentMemory, error) {
	res, err := c.Request(ctx, http.MethodGet, agentMemoriesPath(user)+"/default", nil)
	if err != nil {
		return AgentMemory{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AgentMemory{}, ReadBodyAsError(res)
	}
	var memory AgentMemory
	return memory, json.NewDecoder(res.Body).Decode(&memory)
}

func (c *ExperimentalClient) AgentMemory(ctx context.Context, user string, memoryID uuid.UUID) (AgentMemory, error) {
	res, err := c.Request(ctx, http.MethodGet, agentMemoryPath(user, memoryID), nil)
	if err != nil {
		return AgentMemory{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AgentMemory{}, ReadBodyAsError(res)
	}
	var memory AgentMemory
	return memory, json.NewDecoder(res.Body).Decode(&memory)
}

func (c *ExperimentalClient) UpdateAgentMemory(ctx context.Context, user string, memoryID uuid.UUID, req UpdateAgentMemoryRequest) (AgentMemory, error) {
	res, err := c.Request(ctx, http.MethodPatch, agentMemoryPath(user, memoryID), req)
	if err != nil {
		return AgentMemory{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return AgentMemory{}, ReadBodyAsError(res)
	}
	var memory AgentMemory
	return memory, json.NewDecoder(res.Body).Decode(&memory)
}

func (c *ExperimentalClient) DeleteAgentMemory(ctx context.Context, user string, memoryID uuid.UUID) error {
	res, err := c.Request(ctx, http.MethodDelete, agentMemoryPath(user, memoryID), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
