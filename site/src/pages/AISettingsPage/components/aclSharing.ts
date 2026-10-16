import type { Group, MinimalUser } from "#/api/typesGenerated";
import { getGroupSubtitle, isGroup } from "#/modules/groups";
import type { ACLPrincipal } from "./ACLPrincipalAutocomplete";
import type {
	SharingDialogData,
	SharingPrincipal,
	SharingPrincipalSelection,
} from "./ResourceSharingDialog";

type ResourceACL<Role extends string> = {
	readonly users: readonly (MinimalUser & { readonly role: Role })[];
	readonly groups: readonly (Group & { readonly role: Role })[];
};

const groupPrincipal = (group: Group): SharingPrincipal => ({
	id: group.id,
	name: group.display_name || group.name,
	subtitle: getGroupSubtitle(group),
	avatarUrl: group.avatar_url,
});

const userPrincipal = (user: MinimalUser): SharingPrincipal => ({
	id: user.id,
	name: user.username,
	subtitle: user.name || "User",
	avatarUrl: user.avatar_url,
});

export const sharingDialogData = <Role extends string>(
	acl: ResourceACL<Role>,
): SharingDialogData<Role> => ({
	acl: {
		user_roles: Object.fromEntries(
			acl.users.map((user) => [user.id, user.role]),
		),
		group_roles: Object.fromEntries(
			acl.groups.map((group) => [group.id, group.role]),
		),
	},
	principals: {
		users: Object.fromEntries(
			acl.users.map((user) => [user.id, userPrincipal(user)]),
		),
		groups: Object.fromEntries(
			acl.groups.map((group) => [group.id, groupPrincipal(group)]),
		),
	},
});

export const selectedPrincipal = (
	option: ACLPrincipal,
): SharingPrincipalSelection =>
	isGroup(option)
		? { kind: "group", principal: groupPrincipal(option) }
		: { kind: "user", principal: userPrincipal(option) };
