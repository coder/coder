-- Delete cached Terraform module archives downloaded during the window.
--
-- Template versions affected should be re-imported to fix their dynamic
-- parameters.
--
-- Provisionerd caches the module archive for a template version as a row in
-- `files` (mimetype 'application/x-tar', created_by uuid.Nil) and points
-- `template_version_terraform_values.cached_module_files` at it. Archives are
-- deduplicated on (hash, created_by), so `files.created_at` is the moment that
-- content first entered this database.
--
-- Deleting is safe to do unconditionally: the cache is derived data.
-- Provisionerd re-downloads modules from source and repopulates it on the next
-- template import. The side effect is slower workspace builds
--
-- The foreign key template_version_terraform_values_cached_module_files_fkey is
-- NO ACTION, so references must be cleared before the files rows are removed.
-- Capture the target set first, since clearing the references also destroys the
-- join that identifies it.
CREATE TEMP TABLE identified_module_files ON COMMIT DROP AS
SELECT DISTINCT f.id
FROM files f
		 JOIN template_version_terraform_values tvtv
			  ON tvtv.cached_module_files = f.id
WHERE f.created_by = '00000000-0000-0000-0000-000000000000'
  AND f.mimetype = 'application/x-tar'
  AND f.created_at >= '2026-08-31 08:00:00+00'
  AND f.created_at < '2026-08-31 22:00:00+00';

UPDATE template_version_terraform_values
SET cached_module_files = NULL
WHERE cached_module_files IN (SELECT id FROM identified_module_files);

DELETE FROM files
	USING identified_module_files c
WHERE files.id = c.id;


--
--  A simple SELECT query to show all workspaces that were built
--  from the identified modules
--
-- 	SELECT
-- 		w.name AS workspace,
-- 		u.username AS owner,
-- 		wlb.transition,
-- 		wlb.job_status,
-- 		wlb.created_at
-- 	FROM workspace_latest_builds wlb
-- 			 JOIN workspaces w ON w.id = wlb.workspace_id
-- 			 JOIN users u ON u.id = w.owner_id
-- 	WHERE wlb.template_version_id IN (
-- 		SELECT tvtv.template_version_id
-- 		FROM files f
-- 				 JOIN template_version_terraform_values tvtv
-- 					  ON tvtv.cached_module_files = f.id
-- 		WHERE f.created_by = '00000000-0000-0000-0000-000000000000'
-- 		  AND f.mimetype = 'application/x-tar'
-- 		  AND f.created_at >= '2026-08-31 08:00:00+00'
-- 		  AND f.created_at < '2026-08-31 22:00:00+00'
-- 	)
-- 	ORDER BY w.name;
--
-- A simple SELECT query to show all template versions that were built
-- from the identified modules
--
-- 	SELECT
-- 		t.name  AS template,
-- 		tv.name AS template_version,
-- 		tv.id   AS template_version_id,
-- 		f.id    AS module_file_id,
-- 		f.created_at AS module_cached_at,
-- 		tv.created_at AS version_created_at
-- 	FROM files f
-- 			 JOIN template_version_terraform_values tvtv
-- 				  ON tvtv.cached_module_files = f.id
-- 			 JOIN template_versions tv
-- 				  ON tv.id = tvtv.template_version_id
-- 			 JOIN templates t
-- 				  ON t.id = tv.template_id
-- 	WHERE f.created_by = '00000000-0000-0000-0000-000000000000'
-- 	  AND f.mimetype = 'application/x-tar'
-- 	  AND f.created_at >= '2026-08-31 08:00:00+00'
-- 	  AND f.created_at < '2026-08-31 22:00:00+00'
-- 	ORDER BY t.name, tv.created_at;
