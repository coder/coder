import type { AuthorizationCheck, ChatProject } from "#/api/typesGenerated";

type ChatProjectPermissions = {
	readonly canUpdate: boolean;
	readonly canDelete: boolean;
};

const chatProjectCheckKey = (
	project: Pick<ChatProject, "organization_id" | "created_by">,
	action: "update" | "delete",
) => `${action}:${project.organization_id}:${project.created_by}`;

// Chat project authorization depends only on the organization and creator,
// so checks are keyed by that pair and shared across projects. This also
// keeps the request within the authcheck endpoint's resource_id limits.
export const chatProjectPermissionChecks = (
	projects: readonly Pick<ChatProject, "organization_id" | "created_by">[],
): Record<string, AuthorizationCheck> => {
	const checks: Record<string, AuthorizationCheck> = {};
	for (const project of projects) {
		for (const action of ["update", "delete"] as const) {
			checks[chatProjectCheckKey(project, action)] = {
				object: {
					resource_type: "chat_project",
					organization_id: project.organization_id,
					owner_id: project.created_by,
				},
				action,
			};
		}
	}
	return checks;
};

export const chatProjectPermissionsFor = (
	project: Pick<ChatProject, "organization_id" | "created_by">,
	response: Record<string, boolean> | undefined,
): ChatProjectPermissions => ({
	canUpdate: response?.[chatProjectCheckKey(project, "update")] ?? false,
	canDelete: response?.[chatProjectCheckKey(project, "delete")] ?? false,
});
