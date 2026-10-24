import type {
	ChatProjectACL,
	ChatProjectRole,
	MinimalUser,
} from "#/api/typesGenerated";
import { getGroupSubtitle } from "#/modules/groups";

export const chatProjectRoleLabel: Record<
	Exclude<ChatProjectRole, "">,
	string
> = {
	admin: "Admin",
	use: "Member",
};

type ProjectAccessEntryBase = {
	readonly id: string;
	readonly subtitle: string | undefined;
	readonly avatarUrl: string | undefined;
};

/** One row of a project's access list, shared by the summary and the dialog. */
export type ProjectAccessEntry =
	| (ProjectAccessEntryBase & {
			readonly kind: "owner";
			/** `undefined` while the owner loads. */
			readonly name: string | undefined;
	  })
	| (ProjectAccessEntryBase & {
			readonly kind: "user" | "group";
			readonly name: string;
			readonly role: ChatProjectRole;
	  });

/** `memberCount` includes the owner; groups are counted separately. */
export const formatMembersHeading = (
	memberCount: number,
	groupCount: number,
): string => {
	const members = memberCount === 1 ? "1 member" : `${memberCount} members`;
	if (groupCount === 0) {
		return members;
	}
	return `${members}, ${groupCount === 1 ? "1 group" : `${groupCount} groups`}`;
};

const userSubtitle = (user: MinimalUser) =>
	user.name ? user.username : undefined;

/**
 * Orders the owner first, then users, then groups. The ACL never lists the
 * owner. `owner` is `undefined` while it loads and `null` if it failed to load.
 */
export const buildAccessEntries = (
	ownerId: string,
	owner: MinimalUser | null | undefined,
	acl: ChatProjectACL,
): ProjectAccessEntry[] => [
	{
		kind: "owner",
		id: ownerId,
		name:
			owner === undefined
				? undefined
				: owner === null
					? "Unknown"
					: owner.name || owner.username,
		subtitle: owner ? userSubtitle(owner) : undefined,
		avatarUrl: owner?.avatar_url,
	},
	...acl.users.map(
		(user): ProjectAccessEntry => ({
			kind: "user",
			id: user.id,
			name: user.name || user.username,
			subtitle: userSubtitle(user),
			avatarUrl: user.avatar_url,
			role: user.role,
		}),
	),
	...acl.groups.map(
		(group): ProjectAccessEntry => ({
			kind: "group",
			id: group.id,
			name: group.display_name || group.name,
			subtitle: getGroupSubtitle(group),
			avatarUrl: group.avatar_url,
			role: group.role,
		}),
	),
];
