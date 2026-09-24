import {
	hashKey,
	type UseInfiniteQueryOptions,
	type UseQueryOptions,
} from "react-query";
import { API } from "#/api/api";
import type {
	AIBridgeListSessionsResponse,
	AIBridgeProvider,
	AIBridgeSessionThreadsResponse,
	OrganizationAISpendDetailsFilter,
	OrganizationAISpendFilter,
	OrganizationAISpendReport,
	Pagination,
} from "#/api/typesGenerated";
import { useFilterParamsKey } from "#/components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";
import { permittedOrganizations } from "./organizations";

const SESSION_THREADS_INFINITE_PAGE_SIZE = 20;

export const aiBridgeProviders = (): UseQueryOptions<AIBridgeProvider[]> => ({
	queryKey: ["aiBridgeProviders"],
	queryFn: () => API.getAIBridgeProviders(),
});

export const aiBridgeModels = (
	options: Pagination & { model?: string },
): UseQueryOptions<string[]> => ({
	queryKey: ["aiBridgeModels", options],
	queryFn: () => API.getAIBridgeModels(options),
});

export const aiBridgeClients = (options: {
	q?: string;
	limit?: number;
	offset?: number;
}): UseQueryOptions<string[]> => ({
	queryKey: ["aiBridgeClients", options],
	queryFn: () => API.getAIBridgeClients(options),
});

export const paginatedSessions = (
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<AIBridgeListSessionsResponse, string> => {
	return {
		searchParams,
		queryPayload: () => searchParams.get(useFilterParamsKey) ?? "",
		queryKey: ({ limit, offset, payload }) => {
			return ["aiBridgeSessions", limit, offset, payload] as const;
		},
		queryFn: ({ limit, offset, payload }) =>
			API.getAIBridgeSessionList({
				offset,
				limit,
				q: payload,
			}),
	};
};

// The spend endpoints authorize on reading the organization's group members,
// so this lists exactly the organizations they would serve.
export const aiSpendOrganizations = () =>
	permittedOrganizations({
		object: { resource_type: "group_member" },
		action: "read",
	});

const organizationAISpendScopeKey = (
	organizationId: string,
	filter: OrganizationAISpendFilter,
) => ["organizations", organizationId, "aiSpend", filter] as const;

export const paginatedOrganizationAISpend = (
	organizationId: string,
	filter: OrganizationAISpendFilter,
): UsePaginatedQueryOptions<
	OrganizationAISpendReport,
	OrganizationAISpendFilter
> => {
	return {
		queryPayload: () => filter,
		queryKey: ({ payload, pageNumber }) => [
			...organizationAISpendScopeKey(organizationId, payload),
			pageNumber,
		],
		queryFn: ({ payload, limit, offset }) =>
			API.getOrganizationAISpendUsers(organizationId, {
				...payload,
				limit,
				offset,
			}),
		// Every page aggregates the whole organization window.
		prefetch: false,
		// Rows from another organization or filter must not appear under the
		// new selection while its report loads; only a page change keeps the
		// previous rows behind the refresh overlay.
		placeholderData: (previousData, previousQuery) =>
			previousQuery &&
			hashKey(previousQuery.queryKey.slice(0, -1)) ===
				hashKey(organizationAISpendScopeKey(organizationId, filter))
				? previousData
				: undefined,
	};
};

export const exportOrganizationAISpend = () => ({
	mutationFn: ({
		organizationId,
		filter,
	}: {
		organizationId: string;
		filter: OrganizationAISpendDetailsFilter;
	}) => API.exportOrganizationAISpend(organizationId, filter),
});

export const infiniteSessionThreads = (sessionId: string) => {
	return {
		queryKey: ["aiBridgeSessionThreads", sessionId],
		getNextPageParam: (lastPage: AIBridgeSessionThreadsResponse) => {
			const threads = lastPage.threads;
			if (threads.length < SESSION_THREADS_INFINITE_PAGE_SIZE) {
				return undefined;
			}
			return threads.at(-1)?.id;
		},
		initialPageParam: undefined as string | undefined,
		queryFn: ({ pageParam }) =>
			API.getAIBridgeSessionThreads(sessionId, {
				limit: SESSION_THREADS_INFINITE_PAGE_SIZE,
				after_id: pageParam as string | undefined,
			}),
	} satisfies UseInfiniteQueryOptions<AIBridgeSessionThreadsResponse>;
};
