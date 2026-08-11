CREATE TABLE template_ai_egress_policies (
	template_id uuid NOT NULL REFERENCES templates(id) ON DELETE CASCADE,
	revision bigint NOT NULL,
	rules jsonb NOT NULL DEFAULT '[]',
	created_at timestamptz NOT NULL DEFAULT now(),
	-- This is intentionally not a foreign key so historical attribution survives user cleanup.
	created_by uuid NOT NULL,
	PRIMARY KEY (template_id, revision)
);

COMMENT ON TABLE template_ai_egress_policies IS
	'Insert-only revision history for template AI egress allow policies.';
