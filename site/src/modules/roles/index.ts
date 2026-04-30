import type { SlimRole } from "#/api/typesGenerated";

export type ScopedSlimRole = SlimRole & {
	global?: boolean;
};

export const roleDescriptions: Record<string, string> = {
	owner:
		"Owner can manage all resources, including users, groups, templates, and workspaces.",
	"user-admin": "User admin can manage all users and groups.",
	"template-admin": "Template admin can manage all templates and workspaces.",
	auditor: "Auditor can access the audit logs.",
	"agents-access": "Grants access to Coder Agents chat.",
	member:
		"Everybody is a member. This is a shared and default role for all users.",
};

export const memberRole: ScopedSlimRole = {
	name: "member",
	display_name: "Member",
} as const;

export function getRoleNames(roles: readonly SlimRole[]): string[] {
	return roles.map((role) => role.name);
}

export function combineGlobalAndOrgRoles(
	globalRoles: readonly SlimRole[],
	orgRoles: readonly SlimRole[],
): ScopedSlimRole[] {
	return [
		...globalRoles.map((it) => ({ ...it, global: true })),
		...orgRoles.map((it) => ({ ...it, global: false })),
	];
}

const roleNamesByAccessLevel: readonly string[] = [
	"owner",
	"organization-admin",
	"user-admin",
	"organization-user-admin",
	"template-admin",
	"organization-template-admin",
	"auditor",
	"organization-auditor",
	"agents-access",
	"member",
	"organization-member",
];

export function sortRoles<Role extends SlimRole>(
	roles: readonly Role[],
): readonly Role[] {
	if (roles.length < 2) {
		return roles;
	}

	return [...roles].sort((a, b) => {
		const aAccessLevel = roleNamesByAccessLevel.indexOf(a.name);
		const bAccessLevel = roleNamesByAccessLevel.indexOf(b.name);

		// a is not in the access level list, but b is, so b should come first
		if (aAccessLevel === -1 && bAccessLevel !== -1) {
			return 1;
		}
		// b is not in the access level list, but a is, so a should come first
		if (bAccessLevel === -1 && aAccessLevel !== -1) {
			return -1;
		}
		// Neither is in the access level list, so sort them alphabetically
		if (aAccessLevel === -1 && bAccessLevel === -1) {
			return a.name.localeCompare(b.name);
		}
		// Both are in the access level list, so sort them by access level
		return aAccessLevel - bAccessLevel;
	});
}
