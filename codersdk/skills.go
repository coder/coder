package codersdk

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"
)

// SkillMetadata represents a personal or organization skill without its raw
// Markdown content.
type SkillMetadata struct {
	ID          uuid.UUID `json:"id" format:"uuid"`
	Name        string    `json:"name"`
	Description string    `json:"description"`
	Enabled     bool      `json:"enabled"`
	CreatedAt   time.Time `json:"created_at" format:"date-time"`
	UpdatedAt   time.Time `json:"updated_at" format:"date-time"`
}

// Skill represents a personal or organization skill with its raw Markdown
// content.
type Skill struct {
	SkillMetadata
	Content string `json:"content"`
}

// CreateSkillRequest is the payload for creating a skill.
type CreateSkillRequest struct {
	// Content must be SKILL.md-format Markdown with YAML frontmatter. The
	// frontmatter must include name, may include description, and must be
	// followed by a non-empty body.
	Content string `json:"content"`
}

// UpdateSkillRequest is the payload for updating a skill. At least one field
// must be set.
type UpdateSkillRequest struct {
	// Content must be SKILL.md-format Markdown with YAML frontmatter. The
	// frontmatter must include name, may include description, and must be
	// followed by a non-empty body.
	Content *string `json:"content,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

func userSkillsPath(user string) string {
	return fmt.Sprintf("/api/experimental/users/%s/skills", url.PathEscape(user))
}

func userSkillPath(user string, name string) string {
	return fmt.Sprintf("%s/%s", userSkillsPath(user), url.PathEscape(name))
}

// CreateUserSkill creates a user skill from raw Markdown content.
func (c *ExperimentalClient) CreateUserSkill(ctx context.Context, user string, req CreateSkillRequest) (Skill, error) {
	res, err := c.Request(ctx, http.MethodPost, userSkillsPath(user), req)
	if err != nil {
		return Skill{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusCreated {
		return Skill{}, ReadBodyAsError(res)
	}
	var skill Skill
	return skill, ReadBodyAsJSON(res, &skill)
}

// UserSkills lists user skill metadata for the specified user.
func (c *ExperimentalClient) UserSkills(ctx context.Context, user string) ([]SkillMetadata, error) {
	res, err := c.Request(ctx, http.MethodGet, userSkillsPath(user), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var skills []SkillMetadata
	return skills, ReadBodyAsJSON(res, &skills)
}

// UserSkillByName returns a user skill by name.
func (c *ExperimentalClient) UserSkillByName(ctx context.Context, user string, name string) (Skill, error) {
	res, err := c.Request(ctx, http.MethodGet, userSkillPath(user, name), nil)
	if err != nil {
		return Skill{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Skill{}, ReadBodyAsError(res)
	}
	var skill Skill
	return skill, ReadBodyAsJSON(res, &skill)
}

// UpdateUserSkill updates a user skill's raw Markdown content or enabled state.
func (c *ExperimentalClient) UpdateUserSkill(ctx context.Context, user string, name string, req UpdateSkillRequest) (Skill, error) {
	res, err := c.Request(ctx, http.MethodPatch, userSkillPath(user, name), req)
	if err != nil {
		return Skill{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Skill{}, ReadBodyAsError(res)
	}
	var skill Skill
	return skill, ReadBodyAsJSON(res, &skill)
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

func organizationSkillsPath(organizationID uuid.UUID) string {
	return fmt.Sprintf("/api/experimental/organizations/%s/skills", organizationID)
}

func organizationSkillPath(organizationID uuid.UUID, name string) string {
	return fmt.Sprintf("%s/%s", organizationSkillsPath(organizationID), url.PathEscape(name))
}

// OrganizationSkills lists the organization skill metadata the caller can
// read.
func (c *ExperimentalClient) OrganizationSkills(ctx context.Context, organizationID uuid.UUID) ([]SkillMetadata, error) {
	res, err := c.Request(ctx, http.MethodGet, organizationSkillsPath(organizationID), nil)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, ReadBodyAsError(res)
	}
	var skills []SkillMetadata
	return skills, ReadBodyAsJSON(res, &skills)
}

// OrganizationSkillByName returns an organization skill by name.
func (c *ExperimentalClient) OrganizationSkillByName(ctx context.Context, organizationID uuid.UUID, name string) (Skill, error) {
	res, err := c.Request(ctx, http.MethodGet, organizationSkillPath(organizationID, name), nil)
	if err != nil {
		return Skill{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Skill{}, ReadBodyAsError(res)
	}
	var skill Skill
	return skill, ReadBodyAsJSON(res, &skill)
}
