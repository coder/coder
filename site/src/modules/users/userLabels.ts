import type { ReducedUser } from "#/api/typesGenerated";

type LabeledUser = Pick<
	ReducedUser,
	"username" | "name" | "email" | "is_agent" | "is_service_account"
>;

/**
 * What a user list leads with. An agent user's name is its display name, so
 * it leads with that, and everyone else leads with their username.
 */
export const userListTitle = (user: LabeledUser): string =>
	user.is_agent ? user.name || user.username : user.username;

/** The line under a user's title: their kind, or their email for people. */
export const userListSubtitle = (user: LabeledUser): string => {
	if (user.is_agent) {
		return "Agent";
	}
	if (user.is_service_account) {
		return "Service Account";
	}
	return user.email;
};
