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

// OrganizationSkillRole is a role a user or group holds in an organization
// skill's access control list.
type OrganizationSkillRole string

const (
	OrganizationSkillRoleRead OrganizationSkillRole = "read"
	// OrganizationSkillRoleDeleted removes the principal's ACL entry when used
	// in an update request.
	OrganizationSkillRoleDeleted OrganizationSkillRole = ""
)

// OrganizationSkillACL is the resolved access control list of an
// organization skill.
type OrganizationSkillACL struct {
	Users  []OrganizationSkillUser  `json:"users"`
	Groups []OrganizationSkillGroup `json:"groups"`
}

// OrganizationSkillUser is a user entry in an organization skill ACL.
type OrganizationSkillUser struct {
	MinimalUser
	Role OrganizationSkillRole `json:"role" enums:"read"`
}

// OrganizationSkillGroup is a group entry in an organization skill ACL.
type OrganizationSkillGroup struct {
	Group
	Role OrganizationSkillRole `json:"role" enums:"read"`
}

// UpdateOrganizationSkillACLRequest is a sparse update of an organization
// skill ACL: only the listed principals change, and
// OrganizationSkillRoleDeleted removes an entry.
type UpdateOrganizationSkillACLRequest struct {
	UserRoles  map[string]OrganizationSkillRole `json:"user_roles,omitempty"`
	GroupRoles map[string]OrganizationSkillRole `json:"group_roles,omitempty"`
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

// CreateOrganizationSkill creates an organization skill from raw Markdown
// content.
func (c *ExperimentalClient) CreateOrganizationSkill(ctx context.Context, organizationID uuid.UUID, req CreateSkillRequest) (Skill, error) {
	res, err := c.Request(ctx, http.MethodPost, organizationSkillsPath(organizationID), req)
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

// UpdateOrganizationSkill updates an organization skill's raw Markdown content
// or enabled state.
func (c *ExperimentalClient) UpdateOrganizationSkill(ctx context.Context, organizationID uuid.UUID, name string, req UpdateSkillRequest) (Skill, error) {
	res, err := c.Request(ctx, http.MethodPatch, organizationSkillPath(organizationID, name), req)
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

// DeleteOrganizationSkill deletes an organization skill by name.
func (c *ExperimentalClient) DeleteOrganizationSkill(ctx context.Context, organizationID uuid.UUID, name string) error {
	res, err := c.Request(ctx, http.MethodDelete, organizationSkillPath(organizationID, name), nil)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// OrganizationSkillACL returns the resolved ACL of an organization skill.
func (c *ExperimentalClient) OrganizationSkillACL(ctx context.Context, organizationID uuid.UUID, name string) (OrganizationSkillACL, error) {
	res, err := c.Request(ctx, http.MethodGet, organizationSkillPath(organizationID, name)+"/acl", nil)
	if err != nil {
		return OrganizationSkillACL{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OrganizationSkillACL{}, ReadBodyAsError(res)
	}
	var acl OrganizationSkillACL
	return acl, ReadBodyAsJSON(res, &acl)
}

// UpdateOrganizationSkillACL applies a sparse ACL update to an organization
// skill.
func (c *ExperimentalClient) UpdateOrganizationSkillACL(ctx context.Context, organizationID uuid.UUID, name string, req UpdateOrganizationSkillACLRequest) error {
	res, err := c.Request(ctx, http.MethodPatch, organizationSkillPath(organizationID, name)+"/acl", req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusNoContent {
		return ReadBodyAsError(res)
	}
	return nil
}

// OrganizationSkillACLAvailable returns the organization members and groups
// that can be added to an organization skill ACL.
func (c *ExperimentalClient) OrganizationSkillACLAvailable(ctx context.Context, organizationID uuid.UUID, name string, req UsersRequest) (ACLAvailable, error) {
	res, err := c.Request(ctx, http.MethodGet,
		organizationSkillPath(organizationID, name)+"/acl/available",
		nil,
		req.Pagination.asRequestOption(),
		req.asRequestOption(),
	)
	if err != nil {
		return ACLAvailable{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return ACLAvailable{}, ReadBodyAsError(res)
	}
	var available ACLAvailable
	return available, ReadBodyAsJSON(res, &available)
}
