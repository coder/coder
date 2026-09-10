import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import { users } from "#/api/queries/users";
import type { AIProvider } from "#/api/typesGenerated";
import { Avatar } from "#/components/Avatar/Avatar";
import type { FilterOption } from "#/components/Filter/FilterCombobox/types";
import { AIBridgeClientIcon } from "../icons/AIBridgeClientIcon";
import { AIBridgeModelIcon } from "../icons/AIBridgeModelIcon";
import { AIBridgeProviderIcon } from "../icons/AIBridgeProviderIcon";

// Suggestions are capped; the pickers are searches, not full lists.
const SUGGESTIONS_LIMIT = 25;

/** The slice of `QueryClient` the option loaders depend on. */
export type OptionsQueryClient = Pick<QueryClient, "fetchQuery">;

const matchesQuery = (
	query: string,
	options: readonly FilterOption[],
): FilterOption[] => {
	const normalized = query.trim().toLowerCase();
	if (normalized.length === 0) {
		return [...options];
	}
	return options.filter(
		(option) =>
			option.label.toLowerCase().includes(normalized) ||
			option.value.toLowerCase().includes(normalized),
	);
};

type InitiatorIdentity = Readonly<{ username: string; avatar_url?: string }>;

// The current user's own option. Commits the backend's per-session `me`
// sentinel rather than a static `initiator:<username>`, so a shared link
// resolves to whoever opens it.
const selfInitiatorOption = (me: InitiatorIdentity): FilterOption => ({
	label: `${me.username} (you)`,
	appliedLabel: "me",
	value: "me",
	startIcon: <Avatar fallback={me.username} src={me.avatar_url} size="sm" />,
});

export const getInitiatorFilterOptions = async (
	query: string,
	me: InitiatorIdentity,
	queryClient: OptionsQueryClient,
): Promise<FilterOption[]> => {
	const usersRes = await queryClient.fetchQuery(
		users({ q: query, limit: SUGGESTIONS_LIMIT }),
	);
	const options = usersRes.users
		.filter((user) => user.username !== me.username)
		.map<FilterOption>((user) => ({
			label: user.username,
			value: user.username,
			startIcon: (
				<Avatar fallback={user.username} src={user.avatar_url} size="sm" />
			),
		}));

	const self = selfInitiatorOption(me);
	return [...matchesQuery(query, [self]), ...options];
};

const toProviderOption = (provider: AIProvider): FilterOption => ({
	label: provider.display_name || provider.name,
	value: provider.name,
	startIcon: (
		<AIBridgeProviderIcon provider={provider.type} className="size-icon-sm" />
	),
});

// The providers endpoint takes no search term, so matching happens here.
export const getProviderFilterOptions = async (
	query: string,
): Promise<FilterOption[]> => {
	const providers = await API.experimental.listAIProviders();
	return matchesQuery(query, providers.map(toProviderOption));
};

export const getClientFilterOptions = async (
	query: string,
): Promise<FilterOption[]> => {
	const clients = await API.getAIBridgeClients({
		q: query,
		limit: SUGGESTIONS_LIMIT,
	});
	return clients.map((client) => ({
		label: client,
		value: client,
		startIcon: <AIBridgeClientIcon client={client} className="size-icon-sm" />,
	}));
};

export const getModelFilterOptions = async (
	query: string,
): Promise<FilterOption[]> => {
	const models = await API.getAIBridgeModels({
		q: query,
		limit: SUGGESTIONS_LIMIT,
	});
	return models.map((model) => ({
		label: model,
		value: model,
		startIcon: <AIBridgeModelIcon model={model} className="size-icon-sm" />,
	}));
};
