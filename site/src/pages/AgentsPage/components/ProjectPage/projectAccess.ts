import type {
	ChatProjectACL,
	ChatProjectRole,
	MinimalUser,
} from "#/api/typesGenerated";
import { getGroupSubtitle } from "#/modules/groups";

/** Display labels for project roles. The empty role only removes access in update requests. */
export const chatProjectRoleLabel: Record<
	Exclude<ChatProjectRole, "">,
	string
> = {
	admin: "Admin",
	use: "Member",
};

type ProjectAccessEntryBase = {
	readonly id: string;
	readonly name: string;
	readonly subtitle: string | undefined;
	readonly avatarUrl: string | undefined;
};

/** The owner, a user, or a group with access to a project. */
export type ProjectAccessEntry =
	| (ProjectAccessEntryBase & { readonly kind: "owner" })
	| (ProjectAccessEntryBase & {
			readonly kind: "user" | "group";
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

// The ACL API returns entries in no stable order, so sort them to keep the
// avatar row and dialog from reshuffling between loads.
const compareEntries = (a: ProjectAccessEntry, b: ProjectAccessEntry) =>
	a.name.localeCompare(b.name, undefined, {
		sensitivity: "base",
	}) || a.id.localeCompare(b.id);

/**
 * Orders the owner first, then users, then groups, each sorted by name. The
 * ACL never lists the owner in `users`. `owner` overrides `acl.owner`, such as
 * with the signed-in user when they own the project.
 */
export const buildAccessEntries = (
	acl: ChatProjectACL,
	owner: MinimalUser = acl.owner,
): ProjectAccessEntry[] => [
	{
		kind: "owner",
		id: owner.id,
		name: owner.name || owner.username,
		subtitle: userSubtitle(owner),
		avatarUrl: owner.avatar_url,
	},
	...acl.users
		.map(
			(user): ProjectAccessEntry => ({
				kind: "user",
				id: user.id,
				name: user.name || user.username,
				subtitle: userSubtitle(user),
				avatarUrl: user.avatar_url,
				role: user.role,
			}),
		)
		.sort(compareEntries),
	...acl.groups
		.map(
			(group): ProjectAccessEntry => ({
				kind: "group",
				id: group.id,
				name: group.display_name || group.name,
				subtitle: getGroupSubtitle(group),
				avatarUrl: group.avatar_url,
				role: group.role,
			}),
		)
		.sort(compareEntries),
];
