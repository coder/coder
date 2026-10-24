import type {
	ChatProjectACL,
	ChatProjectRole,
	MinimalUser,
} from "#/api/typesGenerated";
import { getGroupSubtitle } from "#/modules/groups";

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

/** A user or group that can be granted a project role. */
export type ProjectAccessPrincipal = {
	readonly kind: "user" | "group";
	readonly id: string;
};

/** Run once an access change saves, and once it finishes either way. */
export type AccessChangeCallbacks = {
	readonly onSuccess?: () => void;
	readonly onSettled?: () => void;
};

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
		numeric: true,
	}) || a.id.localeCompare(b.id);

/**
 * Orders the owner first, then users, then groups, each sorted by name. The
 * ACL never lists the owner in `users`.
 */
export const buildAccessEntries = (
	acl: ChatProjectACL,
): ProjectAccessEntry[] => [
	{
		kind: "owner",
		id: acl.owner.id,
		name: acl.owner.name || acl.owner.username,
		subtitle: userSubtitle(acl.owner),
		avatarUrl: acl.owner.avatar_url,
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
