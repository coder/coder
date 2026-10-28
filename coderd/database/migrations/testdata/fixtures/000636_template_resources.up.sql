-- Exercise ownership backfill with live state, immutable archives, and job history.
DO $$
DECLARE
    tpl templates%ROWTYPE;
    archive_id uuid;
    policy_id uuid := '06360000-0000-0000-0000-000000000001';
    policy_revision_id uuid := '06360000-0000-0000-0000-000000000002';
    target_id uuid := '06360000-0000-0000-0000-000000000003';
    policy_job_id uuid := '06360000-0000-0000-0000-000000000004';
    resource_job_id uuid := '06360000-0000-0000-0000-000000000005';
    revision_id uuid := '06360000-0000-0000-0000-000000000006';
    run_id uuid := '06360000-0000-0000-0000-000000000007';
BEGIN
    SELECT * INTO STRICT tpl FROM templates
    ORDER BY EXISTS (SELECT 1 FROM workspaces w JOIN workspace_builds b ON b.workspace_id = w.id WHERE w.template_id = templates.id) DESC, id
    LIMIT 1;
    SELECT id INTO STRICT archive_id FROM files ORDER BY id LIMIT 1;
    INSERT INTO provisioner_jobs (id, organization_id, initiator_id, created_at, updated_at, completed_at, provisioner, storage_method, type, input, file_id)
    VALUES
        (policy_job_id, tpl.organization_id, tpl.created_by, now(), now(), now(), 'terraform', 'file', 'network_policy', '{}', archive_id),
        (resource_job_id, tpl.organization_id, tpl.created_by, now(), now(), now(), 'terraform', 'file', 'template_resources', '{}', archive_id);
    INSERT INTO network_policies (id, organization_id, name, created_by, created_at, updated_at)
    VALUES (policy_id, tpl.organization_id, 'migration-fixture-policy', tpl.created_by, now(), now());
    INSERT INTO network_policy_revisions (id, policy_id, organization_id, revision_number, hcl, file_id, created_by, created_at)
    VALUES (policy_revision_id, policy_id, tpl.organization_id, 1, '# immutable policy', archive_id, tpl.created_by, now());
    INSERT INTO network_policy_attachments (policy_id, template_id, organization_id, created_at)
    VALUES (policy_id, tpl.id, tpl.organization_id, now());
    INSERT INTO network_policy_targets (id, policy_id, organization_id, cluster, context, namespace, created_at, updated_at)
    VALUES (target_id, policy_id, tpl.organization_id, 'fixture-cluster', 'fixture-context', 'fixture-namespace', now(), now());
    INSERT INTO network_policy_runs (id, job_id, policy_id, target_id, revision_id, organization_id, generation, template_ids, operation, created_at)
    VALUES (gen_random_uuid(), policy_job_id, policy_id, target_id, policy_revision_id, tpl.organization_id, 1, ARRAY[tpl.id], 'apply', now());
    INSERT INTO template_resource_revisions (id, template_id, organization_id, file_id, source, created_by, created_at)
    VALUES (revision_id, tpl.id, tpl.organization_id, archive_id, '# immutable Foundation root', tpl.created_by, now());
    INSERT INTO template_resource_runs (id, template_id, organization_id, revision_id, job_id, action, created_at)
    VALUES (run_id, tpl.id, tpl.organization_id, revision_id, resource_job_id, 'apply', now());
    INSERT INTO template_resource_states (template_id, organization_id, current_revision_id, applied_revision_id, status, state, outputs, updated_at)
    VALUES (tpl.id, tpl.organization_id, revision_id, revision_id, 'applied', '\x636865636b706f696e74', '{"value":"retained"}', now());
    UPDATE workspace_builds SET template_resource_revision_id = revision_id, template_resource_outputs = '{"value":"retained"}'
    WHERE workspace_id IN (SELECT id FROM workspaces WHERE template_id = tpl.id);
END $$;
