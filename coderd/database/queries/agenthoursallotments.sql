-- name: GetAgentHoursOrganizationAllotments :many
SELECT
	allotment.organization_id,
	allotment.allotment_bps,
	allotment.created_at,
	allotment.updated_at,
	organizations.name AS organization_name,
	organizations.display_name AS organization_display_name
FROM agent_hours_organization_allotments allotment
JOIN organizations ON organizations.id = allotment.organization_id
WHERE organizations.deleted = false
ORDER BY organizations.name ASC;

-- name: GetAgentHoursOrganizationAllotment :one
SELECT *
FROM agent_hours_organization_allotments
WHERE organization_id = @organization_id;

-- name: UpsertAgentHoursOrganizationAllotment :one
INSERT INTO agent_hours_organization_allotments (organization_id, allotment_bps)
VALUES (@organization_id, @allotment_bps)
ON CONFLICT (organization_id) DO UPDATE SET
	allotment_bps = EXCLUDED.allotment_bps,
	updated_at = NOW()
RETURNING *;

-- name: DeleteAgentHoursOrganizationAllotment :one
DELETE FROM agent_hours_organization_allotments
WHERE organization_id = @organization_id
RETURNING *;

-- name: GetAgentHoursGroupAllotmentsByOrganizationID :many
SELECT
	allotment.group_id,
	allotment.allotment_bps,
	allotment.created_at,
	allotment.updated_at,
	groups.name AS group_name,
	groups.display_name AS group_display_name
FROM agent_hours_group_allotments allotment
JOIN groups ON groups.id = allotment.group_id
WHERE groups.organization_id = @organization_id
ORDER BY groups.name ASC;

-- name: UpsertAgentHoursGroupAllotment :one
INSERT INTO agent_hours_group_allotments (group_id, allotment_bps)
VALUES (@group_id, @allotment_bps)
ON CONFLICT (group_id) DO UPDATE SET
	allotment_bps = EXCLUDED.allotment_bps,
	updated_at = NOW()
RETURNING *;

-- name: DeleteAgentHoursGroupAllotment :one
DELETE FROM agent_hours_group_allotments
WHERE group_id = @group_id
RETURNING *;
