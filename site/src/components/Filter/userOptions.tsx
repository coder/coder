import { users } from "#/api/queries/users";
import type { MinimalUser } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import type { OptionsQueryClient } from "#/utils/filters";
import type { FilterOption } from "./FilterCombobox/types";

const USER_SUGGESTIONS_LIMIT = 25;

type UserIdentity = Pick<MinimalUser, "username" | "avatar_url">;

// The backend resolves the `me` sentinel for the current session.
const meUserOption = (me: UserIdentity): FilterOption => ({
	label: `${me.username} (you)`,
	appliedLabel: "me",
	value: "me",
	startIcon: <Avatar fallback={me.username} src={me.avatar_url} size="sm" />,
});

/** Matches the current user's label or the `me` sentinel without fetching users. */
export const getMeUserFilterOptions = async (
	query: string,
	me: UserIdentity,
): Promise<FilterOption[]> => {
	const option = meUserOption(me);
	const normalized = query.trim().toLowerCase();
	if (
		normalized.length === 0 ||
		option.label.toLowerCase().includes(normalized) ||
		option.value.includes(normalized)
	) {
		return [option];
	}
	return [];
};

/**
 * FilterCombobox invokes this loader from a query function, not during render.
 * `fetchQuery` shares the users cache with other consumers.
 */
export const getUserFilterOptions = async (
	query: string,
	me: UserIdentity,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const usersRes = await queryClient.fetchQuery(
		users({
			q: `${query} include_system:true`,
			limit: USER_SUGGESTIONS_LIMIT,
		}),
	);
	// The users API also matches name and email, so a result for the current
	// user keeps its option even when the query is not in its label.
	const meOptions = usersRes.users.some((user) => user.username === me.username)
		? [meUserOption(me)]
		: await getMeUserFilterOptions(query, me);
	const options = usersRes.users
		.filter((user) => user.username !== me.username)
		.map<FilterOption>((user) => ({
			label: user.username,
			value: user.username,
			startIcon: (
				<Avatar fallback={user.username} src={user.avatar_url} size="sm" />
			),
		}));

	return [...meOptions, ...options];
};
