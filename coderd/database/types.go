package database

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/sqlc-dev/pqtype"
	"golang.org/x/xerrors"

	"github.com/coder/coder/v2/coderd/rbac"
	"github.com/coder/coder/v2/coderd/rbac/policy"
)

// AuditOAuthConvertState is never stored in the database. It is stored in a cookie
// clientside as a JWT. This type is provided for audit logging purposes.
type AuditOAuthConvertState struct {
	CreatedAt time.Time `db:"created_at" json:"created_at"`
	// The time at which the state string expires, a merge request times out if the user does not perform it quick enough.
	ExpiresAt     time.Time `db:"expires_at" json:"expires_at"`
	FromLoginType LoginType `db:"from_login_type" json:"from_login_type"`
	// The login type the user is converting to. Should be github or oidc.
	ToLoginType LoginType `db:"to_login_type" json:"to_login_type"`
	UserID      uuid.UUID `db:"user_id" json:"user_id"`
}

type HealthSettings struct {
	ID                    uuid.UUID `db:"id" json:"id"`
	DismissedHealthchecks []string  `db:"dismissed_healthchecks" json:"dismissed_healthchecks"`
}

type NotificationsSettings struct {
	ID             uuid.UUID `db:"id" json:"id"`
	NotifierPaused bool      `db:"notifier_paused" json:"notifier_paused"`
}

type PrebuildsSettings struct {
	ID                   uuid.UUID `db:"id" json:"id"`
	ReconciliationPaused bool      `db:"reconciliation_paused" json:"reconciliation_paused"`
}

type OAuth2ProviderSettings struct {
	ID                               uuid.UUID `db:"id" json:"id"`
	DynamicClientRegistrationEnabled bool      `db:"dynamic_client_registration_enabled" json:"dynamic_client_registration_enabled"`
}

// ChatInstructionSettings is the auditable shape of the deployment-wide
// chat instruction configuration, stored across the
// agents_chat_system_prompt, agents_chat_include_default_system_prompt and
// agents_chat_plan_mode_instructions site_configs keys. Both the
// system-prompt and plan-mode-instructions endpoints audit this one type;
// each populates only the fields its endpoint can change.
type ChatInstructionSettings struct {
	ID uuid.UUID `db:"id" json:"id"`
	// Name identifies which setting an audit row concerns (e.g. "System
	// prompt"). It is ignored in diffs and set identically on Old and New.
	Name         string `db:"name" json:"name"`
	SystemPrompt string `db:"system_prompt" json:"system_prompt"`
	// IncludeDefaultSystemPromptSet records whether the override row
	// exists, not only its effective value: writing explicit false over a
	// legacy absent row does not move the effective value but changes
	// future behavior, so presence must enter the diff.
	IncludeDefaultSystemPromptSet bool   `db:"include_default_system_prompt_set" json:"include_default_system_prompt_set"`
	IncludeDefaultSystemPrompt    bool   `db:"include_default_system_prompt" json:"include_default_system_prompt"`
	PlanModeInstructions          string `db:"plan_mode_instructions" json:"plan_mode_instructions"`
}

// ChatOperationalSettings contains deployment-wide chat settings for audit logging.
type ChatOperationalSettings struct {
	ID                            uuid.UUID `db:"id" json:"id"`
	ChatRetentionDays             string    `db:"chat_retention_days" json:"chat_retention_days"`
	ChatDebugRetentionDays        string    `db:"chat_debug_retention_days" json:"chat_debug_retention_days"`
	ChatAutoArchiveDays           string    `db:"chat_auto_archive_days" json:"chat_auto_archive_days"`
	WorkspaceTTL                  string    `db:"workspace_ttl" json:"workspace_ttl"`
	ComputerUseProvider           string    `db:"computer_use_provider" json:"computer_use_provider"`
	DebugLoggingAllowUsers        string    `db:"debug_logging_allow_users" json:"debug_logging_allow_users"`
	PersonalModelOverridesEnabled string    `db:"personal_model_overrides_enabled" json:"personal_model_overrides_enabled"`
}

type Actions []policy.Action

func (a *Actions) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), &a)
	case []byte:
		return json.Unmarshal(v, &a)
	}
	return xerrors.Errorf("unexpected type %T", src)
}

func (a *Actions) Value() (driver.Value, error) {
	return json.Marshal(a)
}

// TemplateACL is a map of ids to permissions.
type TemplateACL map[string][]policy.Action

func (t *TemplateACL) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), &t)
	case []byte:
		return json.Unmarshal(v, &t)
	case json.RawMessage:
		return json.Unmarshal(v, &t)
	}

	return xerrors.Errorf("unexpected type %T", src)
}

func (t TemplateACL) Value() (driver.Value, error) {
	return json.Marshal(t)
}

type ChatACL map[string]ChatACLEntry

func (c *ChatACL) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), &c)
	case []byte:
		return json.Unmarshal(v, &c)
	case json.RawMessage:
		return json.Unmarshal(v, &c)
	}

	return xerrors.Errorf("unexpected type %T", src)
}

//nolint:revive
func (c ChatACL) RBACACL() map[string][]policy.Action {
	rbacACL := make(map[string][]policy.Action, len(c))
	for id, entry := range c {
		rbacACL[id] = entry.Permissions
	}
	return rbacACL
}

func (c ChatACL) Value() (driver.Value, error) {
	if c == nil {
		return json.Marshal(ChatACL{})
	}
	return json.Marshal(c)
}

type ChatACLEntry struct {
	Permissions []policy.Action `json:"permissions"`
}

// AgentMetadataAggregate is the agent_metadata jsonb array the
// GetWorkspaces query aggregates for the include_agent_metadata
// expansion. Elements have WorkspaceAgentMetadatum's JSON shape; each
// carries its workspace_agent_id so multi-agent workspaces can map
// values onto the right agent. The generated row keeps
// json.RawMessage because sqlc overrides cannot target expression
// columns; callers Scan the raw value into this type.
type AgentMetadataAggregate []WorkspaceAgentMetadatum

func (a *AgentMetadataAggregate) Scan(src interface{}) error {
	switch v := src.(type) {
	case nil:
		return nil
	case string:
		return json.Unmarshal([]byte(v), &a)
	case []byte:
		return json.Unmarshal(v, &a)
	case json.RawMessage:
		return json.Unmarshal(v, &a)
	}

	return xerrors.Errorf("unexpected type %T", src)
}

func (a AgentMetadataAggregate) Value() (driver.Value, error) {
	return json.Marshal(a)
}

type WorkspaceACL map[string]WorkspaceACLEntry

func (t *WorkspaceACL) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), &t)
	case []byte:
		return json.Unmarshal(v, &t)
	case json.RawMessage:
		return json.Unmarshal(v, &t)
	}

	return xerrors.Errorf("unexpected type %T", src)
}

//nolint:revive,staticcheck // Receiver name matches the other WorkspaceACL methods in this file.
func (w WorkspaceACL) RBACACL() map[string][]policy.Action {
	// Convert WorkspaceACL to a map of string to []policy.Action.
	// This is used for RBAC checks.
	rbacACL := make(map[string][]policy.Action, len(w))
	for id, entry := range w {
		rbacACL[id] = entry.Permissions
	}
	return rbacACL
}

func (t WorkspaceACL) Value() (driver.Value, error) {
	return json.Marshal(t)
}

type WorkspaceACLEntry struct {
	Permissions []policy.Action `json:"permissions"`
}

// WorkspaceACLDisplayInfo supplements workspace ACLs with the actors'
// display info.  Key is string rather than uuid.UUID as this aligns
// with how RBAC represents actor IDs.
type WorkspaceACLDisplayInfo map[string]struct {
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// WorkspaceACLDisplayInfo is only used to read from the DB.
func (w *WorkspaceACLDisplayInfo) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), w)
	case []byte:
		return json.Unmarshal(v, w)
	case json.RawMessage:
		return json.Unmarshal(v, w)
	}
	return xerrors.Errorf("unexpected type %T", src)
}

type ExternalAuthProvider struct {
	ID       string `json:"id"`
	Optional bool   `json:"optional,omitempty"`
}

type StringMap map[string]string

func (m *StringMap) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	switch src := src.(type) {
	case []byte:
		err := json.Unmarshal(src, m)
		if err != nil {
			return err
		}
	default:
		return xerrors.Errorf("unsupported Scan, storing driver.Value type %T into type %T", src, m)
	}
	return nil
}

func (m StringMap) Value() (driver.Value, error) {
	return json.Marshal(m)
}

type StringMapOfInt map[string]int64

func (m *StringMapOfInt) Scan(src interface{}) error {
	if src == nil {
		return nil
	}
	switch src := src.(type) {
	case []byte:
		err := json.Unmarshal(src, m)
		if err != nil {
			return err
		}
	default:
		return xerrors.Errorf("unsupported Scan, storing driver.Value type %T into type %T", src, m)
	}
	return nil
}

func (m StringMapOfInt) Value() (driver.Value, error) {
	return json.Marshal(m)
}

type CustomRolePermissions []CustomRolePermission

func (s *APIKeyScopes) Scan(src any) error {
	var arr []string
	if err := pq.Array(&arr).Scan(src); err != nil {
		return err
	}
	out := make(APIKeyScopes, len(arr))
	for i, v := range arr {
		out[i] = APIKeyScope(v)
	}
	*s = out
	return nil
}

func (s APIKeyScopes) Value() (driver.Value, error) {
	arr := make([]string, len(s))
	for i, v := range s {
		arr[i] = string(v)
	}
	return pq.Array(arr).Value()
}

func (a *CustomRolePermissions) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), &a)
	case []byte:
		return json.Unmarshal(v, &a)
	}
	return xerrors.Errorf("unexpected type %T", src)
}

func (a CustomRolePermissions) Value() (driver.Value, error) {
	return json.Marshal(a)
}

type CustomRolePermission struct {
	Negate       bool          `json:"negate"`
	ResourceType string        `json:"resource_type"`
	Action       policy.Action `json:"action"`
}

func (a CustomRolePermission) String() string {
	str := a.ResourceType + "." + string(a.Action)
	if a.Negate {
		return "-" + str
	}
	return str
}

// NameOrganizationPair is used as a lookup tuple for custom role rows.
type NameOrganizationPair struct {
	Name string `db:"name" json:"name"`
	// OrganizationID if unset will assume a null column value
	OrganizationID uuid.UUID `db:"organization_id" json:"organization_id"`
}

func (*NameOrganizationPair) Scan(_ interface{}) error {
	return xerrors.Errorf("this should never happen, type 'NameOrganizationPair' should only be used as a parameter")
}

// Value returns the tuple **literal**
// To get the literal value to return, you can use the expression syntax in a psql
// shell.
//
//		SELECT ('customrole'::text,'ece79dac-926e-44ca-9790-2ff7c5eb6e0c'::uuid);
//	To see 'null' option. Using the nil uuid as null to avoid empty string literals for null.
//		SELECT ('customrole',00000000-0000-0000-0000-000000000000);
//
// This value is usually used as an array, NameOrganizationPair[]. You can see
// what that literal is as well, with proper quoting.
//
//	SELECT ARRAY[('customrole'::text,'ece79dac-926e-44ca-9790-2ff7c5eb6e0c'::uuid)];
func (a NameOrganizationPair) Value() (driver.Value, error) {
	// The string values must be escaped in case there are special characters, quotes, etc.
	// 'NameOrganizationPair' is a composite value, which has no driver handler
	// in the `pq` package.
	//
	// pq.StringArray formats the single name as `{"<escaped>"}`. Strip
	// the outer braces to get the quoted+escaped form that composite
	// literal syntax accepts unchanged.
	//
	// Ideally `appendArrayQuotedBytes` would be exported, and we could call
	// it directly.
	v, err := (&pq.StringArray{a.Name}).Value()
	if err != nil {
		return nil, err
	}

	s, ok := v.(string)
	if !ok {
		return nil, xerrors.Errorf("unexpected type %T", v)
	}

	stripCurlyBraces := s[1 : len(s)-1]

	return fmt.Sprintf("(%s,%s)", stripCurlyBraces, a.OrganizationID.String()), nil
}

// AgentIDNamePair is used as a result tuple for workspace and agent rows.
type AgentIDNamePair struct {
	ID   uuid.UUID `db:"id" json:"id"`
	Name string    `db:"name" json:"name"`
}

func (p *AgentIDNamePair) Scan(src interface{}) error {
	var v string
	switch a := src.(type) {
	case []byte:
		v = string(a)
	case string:
		v = a
	default:
		return xerrors.Errorf("unexpected type %T", src)
	}
	parts := strings.Split(strings.Trim(v, "()"), ",")
	if len(parts) != 2 {
		return xerrors.New("invalid format for AgentIDNamePair")
	}
	id, err := uuid.Parse(strings.TrimSpace(parts[0]))
	if err != nil {
		return err
	}
	p.ID, p.Name = id, strings.TrimSpace(parts[1])
	return nil
}

func (p AgentIDNamePair) Value() (driver.Value, error) {
	return fmt.Sprintf(`(%s,%s)`, p.ID.String(), p.Name), nil
}

// UserLinkClaims is the returned IDP claims for a given user link.
// These claims are fetched at login time. These are the claims that were
// used for IDP sync.
type UserLinkClaims struct {
	IDTokenClaims  map[string]interface{} `json:"id_token_claims"`
	UserInfoClaims map[string]interface{} `json:"user_info_claims"`
	// MergeClaims are computed in Golang. It is the result of merging
	// the IDTokenClaims and UserInfoClaims. UserInfoClaims take precedence.
	MergedClaims map[string]interface{} `json:"merged_claims"`
}

func (a *UserLinkClaims) Scan(src interface{}) error {
	switch v := src.(type) {
	case string:
		return json.Unmarshal([]byte(v), &a)
	case []byte:
		return json.Unmarshal(v, &a)
	}
	return xerrors.Errorf("unexpected type %T", src)
}

func (a UserLinkClaims) Value() (driver.Value, error) {
	return json.Marshal(a)
}

func ParseIP(ipStr string) pqtype.Inet {
	ip := net.ParseIP(ipStr)
	ipNet := net.IPNet{}
	if ip != nil {
		ipNet = net.IPNet{
			IP:   ip,
			Mask: net.CIDRMask(len(ip)*8, len(ip)*8),
		}
	}

	return pqtype.Inet{
		IPNet: ipNet,
		Valid: ip != nil,
	}
}

// AllowList is a typed wrapper around a list of AllowListTarget entries.
// It implements sql.Scanner and driver.Valuer so it can be stored in and
// loaded from a Postgres text[] column that stores each entry in the
// canonical form "type:id".
type AllowList []rbac.AllowListElement

// Scan implements sql.Scanner. It supports inputs that pq.Array can decode
// into []string, and then converts each element to an AllowListTarget.
func (a *AllowList) Scan(src any) error {
	var raw []string
	if err := pq.Array(&raw).Scan(src); err != nil {
		return err
	}
	out := make([]rbac.AllowListElement, len(raw))
	for i, s := range raw {
		e, err := rbac.ParseAllowListEntry(s)
		if err != nil {
			return err
		}
		out[i] = e
	}
	*a = out
	return nil
}

// Value implements driver.Valuer by converting the list to []string using the
// canonical "type:id" form and delegating to pq.Array for encoding.
func (a AllowList) Value() (driver.Value, error) {
	raw := make([]string, len(a))
	for i, t := range a {
		raw[i] = t.String()
	}
	return pq.Array(raw).Value()
}
