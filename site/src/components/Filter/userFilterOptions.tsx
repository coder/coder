import type { QueryClient } from "react-query";
import { users } from "#/api/queries/users";
import { Avatar } from "#/components/Avatar/Avatar";
import type { FilterOption } from "#/components/Filter/FilterCombobox/types";

// User suggestions are capped; the picker is a prefix search, not a full list.
const USER_SUGGESTIONS_LIMIT = 25;

/** The slice of `QueryClient` the option loaders depend on. */
export type OptionsQueryClient = Pick<QueryClient, "fetchQuery">;

type UserIdentity = Readonly<{ username: string; avatar_url?: string }>;

// The current user's own option. Commits the backend's per-session `me`
// sentinel rather than a static username.
const selfUserOption = (me: UserIdentity): FilterOption => ({
	label: `${me.username} (you)`,
	value: "me",
	startIcon: <Avatar fallback={me.username} src={me.avatar_url} size="md" />,
});

// Users who cannot list other users still filter by themselves, so the user
// category stays available (and its chip key stays recognized) with just the
// "you" option.
export const getSelfUserFilterOptions = async (
	query: string,
	me: UserIdentity,
): Promise<FilterOption[]> => {
	const option = selfUserOption(me);
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

export const getUserFilterOptions = async (
	query: string,
	me: UserIdentity,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const usersRes = await queryClient.fetchQuery(
		users({ q: query, limit: USER_SUGGESTIONS_LIMIT }),
	);
	const options = usersRes.users
		.filter((user) => user.username !== me.username)
		.map<FilterOption>((user) => ({
			label: user.username,
			value: user.username,
			startIcon: (
				<Avatar fallback={user.username} src={user.avatar_url} size="md" />
			),
		}));

	return [selfUserOption(me), ...options];
};
