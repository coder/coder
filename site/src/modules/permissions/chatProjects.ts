import type { AuthorizationCheck, ChatProject } from "#/api/typesGenerated";

type ChatProjectPermissions = {
	readonly canUpdate: boolean;
	readonly canDelete: boolean;
};

type ChatProjectAction = "update" | "delete";

const chatProjectActions: readonly ChatProjectAction[] = ["update", "delete"];

const chatProjectCheckKey = (
	project: Pick<ChatProject, "organization_id" | "owner_id">,
	action: ChatProjectAction,
) => `${action}:${project.organization_id}:${project.owner_id}`;

// Chat project authorization depends only on the organization and owner,
// so checks are keyed by that pair and shared across projects. This also
// keeps the request within the authcheck endpoint's resource_id limits.
export const chatProjectPermissionChecks = (
	projects: readonly Pick<ChatProject, "organization_id" | "owner_id">[],
): Record<string, AuthorizationCheck> => {
	const checks: Record<string, AuthorizationCheck> = {};
	for (const project of projects) {
		for (const action of chatProjectActions) {
			checks[chatProjectCheckKey(project, action)] = {
				object: {
					resource_type: "chat_project",
					organization_id: project.organization_id,
					owner_id: project.owner_id,
				},
				action,
			};
		}
	}
	return checks;
};

export const chatProjectPermissionsFor = (
	project: Pick<ChatProject, "organization_id" | "owner_id">,
	response: Record<string, boolean> | undefined,
): ChatProjectPermissions => ({
	canUpdate: response?.[chatProjectCheckKey(project, "update")] ?? false,
	canDelete: response?.[chatProjectCheckKey(project, "delete")] ?? false,
});
