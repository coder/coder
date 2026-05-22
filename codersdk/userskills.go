package codersdk

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

// UserSkillMetadata represents a user skill without its raw Markdown content.
type UserSkillMetadata struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	CreatedAt   time.Time `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time `json:"updated_at" format:"date-time"`
}

// UserSkill represents a user skill with its raw Markdown content.
type UserSkill struct {
	UserSkillMetadata
	Content string `json:"content"`
}

// CreateUserSkillRequest is the payload for creating a user skill.
type CreateUserSkillRequest struct {
	// Content must be SKILL.md-format Markdown with YAML frontmatter. The
	// frontmatter must include name, may include description, and must be
	// followed by a non-empty body.
	Content string `json:"content"`
}

// UpdateUserSkillRequest is the payload for updating a user skill.
type UpdateUserSkillRequest struct {
	// Content must be SKILL.md-format Markdown with YAML frontmatter. The
	// frontmatter must include name, may include description, and must be
	// followed by a non-empty body.
	Content string `json:"content"`
}

func userSkillsPath(user string) string {
	return fmt.Sprintf("/api/experimental/users/%s/skills", url.PathEscape(user))
}

func userSkillPath(user string, name string) string {
	return fmt.Sprintf("%s/%s", userSkillsPath(user), url.PathEscape(name))
}

// CreateUserSkill creates a user skill from raw Markdown content.
func (c *ExperimentalClient) CreateUserSkill(ctx context.Context, user string, req CreateUserSkillRequest) (UserSkill, error) {
	res, err := c.Request(ctx, http.MethodPost, userSkillsPath(user), req)
	if err != nil {
		return UserSkill{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return UserSkill{}, ReadBodyAsError(res)
	}
	var skill UserSkill
	return skill, json.NewDecoder(res.Body).Decode(&skill)
}

// UserSkills lists user skill metadata for the specified user.
func (c *ExperimentalClient) UserSkills(ctx context.Context, user string) ([]UserSkillMetadata, error) {
	res, err := c.Request(ctx, http.MethodGet, userSkillsPath(user), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var skills []UserSkillMetadata
	return skills, json.NewDecoder(res.Body).Decode(&skills)
}

// UserSkillByName returns a user skill by name.
func (c *ExperimentalClient) UserSkillByName(ctx context.Context, user string, name string) (UserSkill, error) {
	res, err := c.Request(ctx, http.MethodGet, userSkillPath(user, name), nil)
	if err != nil {
		return UserSkill{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UserSkill{}, ReadBodyAsError(res)
	}
	var skill UserSkill
	return skill, json.NewDecoder(res.Body).Decode(&skill)
}

// UpdateUserSkill replaces a user skill's raw Markdown content.
func (c *ExperimentalClient) UpdateUserSkill(ctx context.Context, user string, name string, req UpdateUserSkillRequest) (UserSkill, error) {
	res, err := c.Request(ctx, http.MethodPatch, userSkillPath(user, name), req)
	if err != nil {
		return UserSkill{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return UserSkill{}, ReadBodyAsError(res)
	}
	var skill UserSkill
	return skill, json.NewDecoder(res.Body).Decode(&skill)
}

// DeleteUserSkill deletes a user skill by name.
func (c *ExperimentalClient) DeleteUserSkill(ctx context.Context, user string, name string) error {
	res, err := c.Request(ctx, http.MethodDelete, userSkillPath(user, name), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}
