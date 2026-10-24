import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type {
	CreateGitHubRunnerPoolRequest,
	UpdateGitHubRunnerPoolRequest,
} from "#/api/typesGenerated";

/** Scopes pool lists and descendant allocation queries to one organization. */
export const githubRunnerPoolsKey = (organization: string) =>
	["organizations", organization, "github-runner-pools"] as const;

/** Separates allocation history for each pool within its organization. */
export const githubRunnersKey = (organization: string, pool: string) =>
	[...githubRunnerPoolsKey(organization), pool, "runners"] as const;

/** Refreshes controller status while the pool management page is open. */
export const githubRunnerPools = (organization: string) => ({
	queryKey: githubRunnerPoolsKey(organization),
	queryFn: () => API.getGitHubRunnerPools(organization),
	refetchInterval: 5000,
});

/** Refreshes allocation lifecycle observations independently of pool configuration. */
export const githubRunners = (organization: string, pool: string) => ({
	queryKey: githubRunnersKey(organization, pool),
	queryFn: () => API.getGitHubRunners(organization, pool),
	refetchInterval: 5000,
});

/** Invalidates organization data after creation and discards inactive mutations containing a PAT. */
export const createGitHubRunnerPool = (
	queryClient: QueryClient,
	organization: string,
) => ({
	mutationFn: (request: CreateGitHubRunnerPoolRequest) =>
		API.createGitHubRunnerPool(organization, request),
	onSuccess: () =>
		queryClient.invalidateQueries({
			queryKey: githubRunnerPoolsKey(organization),
			exact: true,
		}),
	gcTime: 0,
});

/** Refreshes pool and allocation status after capacity or lifecycle changes. */
export const updateGitHubRunnerPool = (
	queryClient: QueryClient,
	organization: string,
) => ({
	mutationFn: ({
		pool,
		request,
	}: {
		pool: string;
		request: UpdateGitHubRunnerPoolRequest;
	}) => API.updateGitHubRunnerPool(organization, pool, request),
	onSuccess: async (_: unknown, { pool }: { pool: string }) => {
		await Promise.all([
			queryClient.invalidateQueries({
				queryKey: githubRunnerPoolsKey(organization),
				exact: true,
			}),
			queryClient.invalidateQueries({
				queryKey: githubRunnersKey(organization, pool),
				exact: true,
			}),
		]);
	},
});

/** Starts draining and refreshes the pool status until deletion completes. */
export const deleteGitHubRunnerPool = (
	queryClient: QueryClient,
	organization: string,
) => ({
	mutationFn: (pool: string) => API.deleteGitHubRunnerPool(organization, pool),
	onSuccess: async (_: unknown, pool: string) => {
		await Promise.all([
			queryClient.invalidateQueries({
				queryKey: githubRunnerPoolsKey(organization),
				exact: true,
			}),
			queryClient.invalidateQueries({
				queryKey: githubRunnersKey(organization, pool),
				exact: true,
			}),
		]);
	},
});
