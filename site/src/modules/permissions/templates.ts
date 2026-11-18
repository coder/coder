import type { AuthorizationCheck } from "#/api/typesGenerated";

/** Resource-scoped permissions used by template pages and actions. */
export const templatePermissionChecks = (templateId: string) =>
	({
		canUpdateTemplate: {
			object: { resource_type: "template", resource_id: templateId },
			action: "update",
		},
		canDeleteTemplate: {
			object: { resource_type: "template", resource_id: templateId },
			action: "delete",
		},
		canReadInsights: {
			object: { resource_type: "template", resource_id: templateId },
			action: "view_insights",
		},
	}) satisfies Record<string, AuthorizationCheck>;

/** Authorization results for checks scoped to a template. */
export type TemplatePermissions = Record<
	keyof ReturnType<typeof templatePermissionChecks>,
	boolean
>;
