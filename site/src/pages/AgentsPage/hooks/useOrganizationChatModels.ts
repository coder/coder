import { useQueries } from "react-query";
import { chatModels } from "#/api/queries/chats";

/**
 * Aggregates chat models across the given organizations.
 *
 * Each model endpoint applies its model ACL. Any request failure is reported
 * through partialError while cached data is present, so callers keep showing
 * cached models during a failed background refetch. isLoading stays true until
 * every model request settles.
 */
export const useOrganizationChatModels = (
	organizationIds: readonly string[],
) => {
	const queries = useQueries({
		queries: organizationIds.map((organizationId) =>
			chatModels(organizationId),
		),
	});

	const modelError = queries.find((query) => query.error)?.error ?? null;
	const hasData = queries.some((query) => query.data !== undefined);

	return {
		models: queries.flatMap((query) => query.data?.models ?? []),
		isLoading: queries.some((query) => query.isLoading),
		isFetching: queries.some((query) => query.isFetching),
		error: hasData ? null : modelError,
		partialError: hasData ? modelError : null,
	};
};
