package database

// Constraint names raised by the per-user cap trigger functions on
// user_secrets (migration 000509) and user_skills (migration 000502),
// serialized per user by advisory locks since migration 000590.
// RAISE ... USING CONSTRAINT names never appear in pg_constraint, so the
// generated check_constraint.go cannot own them; they are declared once
// here and matched by the API handlers and by the cap tests, which pin
// each name with a real failing write.
const (
	// CheckUserSecretsPerUserCountLimit rejects inserts beyond the per-user
	// secret count cap.
	//nolint:gosec // A trigger constraint name, not a credential.
	CheckUserSecretsPerUserCountLimit CheckConstraint = "user_secrets_per_user_count_limit"
	// CheckUserSecretsPerUserTotalBytesLimit rejects writes that would push
	// the per-user total secret value bytes over the cap.
	//nolint:gosec // A trigger constraint name, not a credential.
	CheckUserSecretsPerUserTotalBytesLimit CheckConstraint = "user_secrets_per_user_total_bytes_limit"
	// CheckUserSecretsPerUserEnvBytesLimit rejects writes that would push
	// the per-user env-injected secret value bytes over the cap.
	//nolint:gosec // A trigger constraint name, not a credential.
	CheckUserSecretsPerUserEnvBytesLimit CheckConstraint = "user_secrets_per_user_env_bytes_limit"
	// CheckUserSkillsPerUserLimit rejects inserts and owner reassignments
	// beyond the per-user skill count cap.
	CheckUserSkillsPerUserLimit CheckConstraint = "user_skills_per_user_limit"
)
