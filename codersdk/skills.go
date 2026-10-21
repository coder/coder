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

// OrganizationSkillSettings is an organization's member opt-out setting for
// organization skills.
type OrganizationSkillSettings struct {
	// MembersCanDisableSkills lets each organization member turn off
	// organization skills for their own chats.
	MembersCanDisableSkills bool `json:"members_can_disable_skills"`
}

// UpdateOrganizationSkillSettingsRequest is the payload for updating an
// organization's member opt-out setting for organization skills.
type UpdateOrganizationSkillSettingsRequest struct {
	MembersCanDisableSkills bool `json:"members_can_disable_skills"`
}

// OrganizationSkillPreferences is an organization member's choices about the
// organization skills that load into their chats. The choices apply only
// while MembersCanDisableSkills is true.
type OrganizationSkillPreferences struct {
	// MembersCanDisableSkills is the organization's setting.
	MembersCanDisableSkills bool `json:"members_can_disable_skills"`
	// AllSkillsDisabled turns off every organization skill, including
	// skills added later. DisabledSkillIDs is kept and applies again when it
	// is false.
	AllSkillsDisabled bool `json:"all_skills_disabled"`
	// DisabledSkillIDs lists the organization skills the member turned off.
	DisabledSkillIDs []uuid.UUID `json:"disabled_skill_ids" format:"uuid"`
}

// UpdateOrganizationSkillPreferencesRequest is a sparse update of an
// organization member's organization skill choices. At least one field must
// be set, and only the listed skills change.
type UpdateOrganizationSkillPreferencesRequest struct {
	AllSkillsDisabled *bool `json:"all_skills_disabled,omitempty"`
	// Skills maps an organization skill ID to whether the skill is enabled
	// for the member's chats.
	Skills map[string]bool `json:"skills,omitempty"`
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

func organizationSkillSettingsPath(organizationID uuid.UUID) string {
	return fmt.Sprintf("/api/experimental/organizations/%s/skill-settings", organizationID)
}

func organizationSkillPreferencesPath(organizationID uuid.UUID, user string) string {
	return fmt.Sprintf("/api/experimental/organizations/%s/members/%s/skill-preferences", organizationID, url.PathEscape(user))
}

// OrganizationSkillSettings returns an organization's member opt-out setting
// for organization skills.
func (c *ExperimentalClient) OrganizationSkillSettings(ctx context.Context, organizationID uuid.UUID) (OrganizationSkillSettings, error) {
	res, err := c.Request(ctx, http.MethodGet, organizationSkillSettingsPath(organizationID), nil)
	if err != nil {
		return OrganizationSkillSettings{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OrganizationSkillSettings{}, ReadBodyAsError(res)
	}
	var settings OrganizationSkillSettings
	return settings, ReadBodyAsJSON(res, &settings)
}

// UpdateOrganizationSkillSettings updates an organization's member opt-out
// setting for organization skills.
func (c *ExperimentalClient) UpdateOrganizationSkillSettings(ctx context.Context, organizationID uuid.UUID, req UpdateOrganizationSkillSettingsRequest) (OrganizationSkillSettings, error) {
	res, err := c.Request(ctx, http.MethodPut, organizationSkillSettingsPath(organizationID), req)
	if err != nil {
		return OrganizationSkillSettings{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OrganizationSkillSettings{}, ReadBodyAsError(res)
	}
	var settings OrganizationSkillSettings
	return settings, ReadBodyAsJSON(res, &settings)
}

// OrganizationSkillPreferences returns an organization member's organization
// skill choices.
func (c *ExperimentalClient) OrganizationSkillPreferences(ctx context.Context, organizationID uuid.UUID, user string) (OrganizationSkillPreferences, error) {
	res, err := c.Request(ctx, http.MethodGet, organizationSkillPreferencesPath(organizationID, user), nil)
	if err != nil {
		return OrganizationSkillPreferences{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OrganizationSkillPreferences{}, ReadBodyAsError(res)
	}
	var prefs OrganizationSkillPreferences
	return prefs, ReadBodyAsJSON(res, &prefs)
}

// UpdateOrganizationSkillPreferences applies a sparse update to an
// organization member's organization skill choices.
func (c *ExperimentalClient) UpdateOrganizationSkillPreferences(ctx context.Context, organizationID uuid.UUID, user string, req UpdateOrganizationSkillPreferencesRequest) (OrganizationSkillPreferences, error) {
	res, err := c.Request(ctx, http.MethodPatch, organizationSkillPreferencesPath(organizationID, user), req)
	if err != nil {
		return OrganizationSkillPreferences{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return OrganizationSkillPreferences{}, ReadBodyAsError(res)
	}
	var prefs OrganizationSkillPreferences
	return prefs, ReadBodyAsJSON(res, &prefs)
}

func chatProjectSkillPath(organizationID, projectID uuid.UUID, name string) string {
	return chatProjectPath(organizationID, projectID) + "/skills/" + url.PathEscape(name)
}

// CreateChatProjectSkill creates a chat project skill from raw Markdown
// content.
func (c *ExperimentalClient) CreateChatProjectSkill(ctx context.Context, organizationID, projectID uuid.UUID, req CreateSkillRequest) (Skill, error) {
	res, err := c.Request(ctx, http.MethodPost, chatProjectPath(organizationID, projectID)+"/skills", req)
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

// ChatProjectSkills lists a chat project's skill metadata.
func (c *ExperimentalClient) ChatProjectSkills(ctx context.Context, organizationID, projectID uuid.UUID) ([]SkillMetadata, error) {
	res, err := c.Request(ctx, http.MethodGet, chatProjectPath(organizationID, projectID)+"/skills", nil)
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

// ChatProjectSkill returns a chat project skill by name.
func (c *ExperimentalClient) ChatProjectSkill(ctx context.Context, organizationID, projectID uuid.UUID, name string) (Skill, error) {
	res, err := c.Request(ctx, http.MethodGet, chatProjectSkillPath(organizationID, projectID, name), nil)
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

// UpdateChatProjectSkill updates a chat project skill's raw Markdown content
// or enabled state.
func (c *ExperimentalClient) UpdateChatProjectSkill(ctx context.Context, organizationID, projectID uuid.UUID, name string, req UpdateSkillRequest) (Skill, error) {
	res, err := c.Request(ctx, http.MethodPatch, chatProjectSkillPath(organizationID, projectID, name), req)
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

// DeleteChatProjectSkill deletes a chat project skill by name.
func (c *ExperimentalClient) DeleteChatProjectSkill(ctx context.Context, organizationID, projectID uuid.UUID, name string) error {
	res, err := c.Request(ctx, http.MethodDelete, chatProjectSkillPath(organizationID, projectID, name), nil)
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
