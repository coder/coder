DROP TABLE network_policy_runs;
DROP TABLE network_policy_targets;
DROP TABLE network_policy_attachments;
ALTER TABLE network_policies DROP CONSTRAINT network_policies_published_revision_fkey;
DROP TABLE network_policy_revisions;
DROP TABLE network_policies;
ALTER TABLE provisioner_jobs DROP CONSTRAINT provisioner_jobs_id_organization_id_key;
ALTER TABLE templates DROP CONSTRAINT templates_id_organization_id_key;
-- PostgreSQL cannot remove an enum value. Retain network_policy so existing
-- provisioner job history remains readable after a rollback.
