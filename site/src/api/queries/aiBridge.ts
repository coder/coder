import {
	hashKey,
	type QueryClient,
	queryOptions,
	type UseInfiniteQueryOptions,
} from "react-query";
import { API } from "#/api/api";
import type {
	AIBridgeListSessionsResponse,
	AIBridgeProvider,
	AIBridgeSessionThreadsResponse,
	AISpendPeriodWindow,
	OrganizationAISpendFilter,
	OrganizationAISpendReport,
	Pagination,
} from "#/api/typesGenerated";
import { useFilterParamsKey } from "#/components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";
import { group } from "./groups";
import { permittedOrganizations } from "./organizations";
import { user } from "./users";

const SESSION_THREADS_INFINITE_PAGE_SIZE = 20;

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

export const aiBridgeProviders = () => ({
	queryKey: ["aiBridgeProviders"],
	queryFn: (): Promise<AIBridgeProvider[]> => API.getAIBridgeProviders(),
});

export const aiBridgeModels = (options: Pagination & { model?: string }) => ({
	queryKey: ["aiBridgeModels", options],
	queryFn: () => API.getAIBridgeModels(options),
});

export const aiBridgeClients = (options: Pagination & { q?: string }) => ({
	queryKey: ["aiBridgeClients", options],
	queryFn: () => API.getAIBridgeClients(options),
});

// The retention only changes when the deployment restarts.
export const aiGatewayRetention = () => ({
	queryKey: ["aiGatewayRetention"],
	queryFn: () => API.experimental.getAIGatewayRetention(),
	staleTime: Number.POSITIVE_INFINITY,
});

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
			API.experimental.getOrganizationAISpendUsers(organizationId, {
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

/**
 * Filters a detailed AI spend report by provider, model, client, user, and
 * group. Users and groups are given by name, which the report resolves to the
 * IDs the endpoint filters by.
 */
export type AISpendReportFilter = Pick<
	OrganizationAISpendFilter,
	"provider_name" | "model" | "client"
> & {
	username?: string;
	group_name?: string;
};

/** Resolves the username and group name to the IDs the spend endpoints filter by. */
const resolveAISpendReportFilter = async (
	queryClient: QueryClient,
	organizationId: string,
	{ username, group_name, ...dimensions }: AISpendReportFilter,
): Promise<OrganizationAISpendFilter> => {
	const [resolvedUser, resolvedGroup] = await Promise.all([
		username ? queryClient.fetchQuery(user(username)) : undefined,
		group_name
			? queryClient.fetchQuery(
					group(organizationId, group_name, { exclude_members: true }),
				)
			: undefined,
	]);
	return {
		...dimensions,
		user_ids: resolvedUser && [resolvedUser.id],
		group_id: resolvedGroup?.id,
	};
};

/** Exports the organization's AI spend for the period as CSV. */
export const exportOrganizationAISpend = (queryClient: QueryClient) => ({
	mutationFn: async ({
		organizationId,
		period,
		filter,
	}: {
		organizationId: string;
		period: AISpendPeriodWindow;
		filter: AISpendReportFilter;
	}): Promise<Blob> => {
		// TODO(AIGOV-733): export the client filter once the endpoint supports it.
		const { user_ids, group_id, provider_name, model } =
			await resolveAISpendReportFilter(queryClient, organizationId, filter);
		return API.getOrganizationAISpendExport(organizationId, {
			...period,
			user_id: user_ids?.[0],
			group_id,
			provider_name,
			model,
		});
	},
});

export const organizationAISpendReportsScopeKey = (organizationId: string) =>
	["organizations", organizationId, "aiSpendReport"] as const;

const organizationAISpendReportKey = (
	organizationId: string,
	period: AISpendPeriodWindow,
	filter: AISpendReportFilter,
) =>
	[
		...organizationAISpendReportsScopeKey(organizationId),
		period,
		filter,
	] as const;

export const paginatedOrganizationAISpendReport = (
	queryClient: QueryClient,
	organizationId: string,
	period: AISpendPeriodWindow,
	filter: AISpendReportFilter,
): UsePaginatedQueryOptions<
	OrganizationAISpendReport,
	AISpendReportFilter,
	Error
> => {
	return {
		queryPayload: () => filter,
		queryKey: ({ payload, pageNumber }) => [
			...organizationAISpendReportKey(organizationId, period, payload),
			pageNumber,
		],
		queryFn: async ({ payload, limit, offset, signal }) =>
			API.experimental.getOrganizationAISpendUsers(
				organizationId,
				{
					...period,
					...(await resolveAISpendReportFilter(
						queryClient,
						organizationId,
						payload,
					)),
					limit,
					offset,
				},
				signal,
			),
		// Each page totals the whole period on the server, so skip loading the
		// neighbouring pages in the background.
		prefetch: false,
		// While the next page loads, keep the current rows. A different filter,
		// period, or organization is a new report, so it shows the loading state.
		placeholderData: (previousData, previousQuery) =>
			previousQuery &&
			hashKey(previousQuery.queryKey.slice(0, -1)) ===
				hashKey(organizationAISpendReportKey(organizationId, period, filter))
				? previousData
				: undefined,
	};
};

/**
 * The organization's AI spend over `period`, in whole UTC days, with the
 * first `limit` users by spend.
 */
export const organizationAISpendOverview = (
	organizationId: string,
	period: AISpendPeriodWindow,
	limit: number,
) =>
	queryOptions({
		queryKey: [
			"organizations",
			organizationId,
			"aiSpendOverview",
			period,
			limit,
		],
		queryFn: () =>
			API.experimental.getOrganizationAISpendOverview(
				organizationId,
				period,
				limit,
			),
	});

/**
 * The given users' spend over `period`, broken down by provider, model, and
 * client.
 */
export const organizationAISpendUsers = (
	organizationId: string,
	period: AISpendPeriodWindow,
	userIds: readonly string[],
) =>
	queryOptions({
		queryKey: [
			"organizations",
			organizationId,
			"aiSpendUsers",
			period,
			userIds,
		],
		queryFn: () =>
			API.experimental.getOrganizationAISpendUsers(organizationId, {
				...period,
				user_ids: userIds,
				limit: userIds.length,
			}),
	});

/**
 * The models used in the period that still have no price, most unpriced tokens
 * first, up to the given limit.
 */
export const organizationUnpricedAIModels = (
	organizationId: string,
	period: AISpendPeriodWindow,
	limit: number,
) =>
	queryOptions({
		queryKey: [
			"organizations",
			organizationId,
			"unpricedAIModels",
			period,
			limit,
		],
		queryFn: () =>
			API.experimental.getOrganizationUnpricedAIModels(
				organizationId,
				period,
				limit,
			),
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
