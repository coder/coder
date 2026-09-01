import {
	type Group,
	MaxAISpendLimitMicros,
	type OrganizationMemberWithUserData,
	type ReducedUser,
	type User,
	type WorkspaceUser,
} from "#/api/typesGenerated";
import { MICROS_PER_DOLLAR, usdBudgetFormatter } from "#/utils/currency";

/** Highest AI budget that can be configured for a group or member, in dollars. */
export const maxAIBudgetDollars = MaxAISpendLimitMicros / MICROS_PER_DOLLAR;

/** Shown when an entered AI budget falls outside the configurable range. */
export const aiBudgetRangeError = `Enter an amount between 0 and ${usdBudgetFormatter.format(maxAIBudgetDollars)}.`;

/**
 * Union of all user-like types that can be distinguished from Group.
 */
type UserLike =
	| User
	| ReducedUser
	| WorkspaceUser
	| OrganizationMemberWithUserData;

/**
 * Type guard to check if the value is a Group.
 * Groups have a "members" property that users don't have.
 */
export const isGroup = (value: UserLike | Group): value is Group => {
	return "members" in value;
};

/**
 * Returns true if the provided group is the 'Everyone' group.
 * The everyone group represents all the users in an organization
 * for which every organization member is implicitly a member of.
 *
 * @param {Group} group - The group to evaluate.
 * @returns {boolean} - Returns true if the group's ID matches its
 * organization ID.
 */
export const isEveryoneGroup = (group: Group): boolean =>
	group.id === group.organization_id;

export const getGroupSubtitle = (group: Group): string => {
	// It is the everyone group when a group id is the same of the org id
	if (group.id === group.organization_id) {
		return "All users";
	}

	const total = group.total_member_count ?? group.members?.length ?? 0;

	if (total === 1) {
		return "1 member";
	}

	return `${total} members`;
};
