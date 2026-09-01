package database

// Constraint names raised by the user soft-delete guard trigger functions
// installed by migration 000591 (check_user_not_deleted and the per-table
// functions delegating to it). These are raised with USING CONSTRAINT from
// plpgsql, not declared as table CHECK constraints, so dbgen does not emit
// them in check_constraint.go; they are declared once here so handlers and
// tests share one set of literals. TestSoftDeleteGuardWinsConcurrentInsert
// pins each name against the live trigger by matching the constraint a real
// failing insert raises.
const (
	// Raised by the guard triggers when inserting a child row for a
	// soft-deleted user, reassigning a child row onto one, or (for the
	// upsert tables user_links, user_secrets, and user_skills) updating a
	// surviving child row of one.
	//nolint:gosec // A trigger constraint name, not a credential.
	CheckAPIKeyUserDeleted               CheckConstraint = "api_key_user_deleted"
	CheckUserLinkUserDeleted             CheckConstraint = "user_link_user_deleted"
	CheckUserSecretUserDeleted           CheckConstraint = "user_secret_user_deleted"
	CheckUserSkillUserDeleted            CheckConstraint = "user_skill_user_deleted"
	CheckUserAIProviderKeyUserDeleted    CheckConstraint = "user_ai_provider_key_user_deleted"
	CheckOrganizationMemberUserDeleted   CheckConstraint = "organization_member_user_deleted"
	CheckUserAIBudgetOverrideUserDeleted CheckConstraint = "user_ai_budget_override_user_deleted"
	CheckGroupMemberUserDeleted          CheckConstraint = "group_member_user_deleted"
)
