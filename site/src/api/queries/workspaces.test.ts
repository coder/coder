import { QueryClient } from "react-query";
import { describe, expect, it } from "vitest";
import type { WorkspacesResponse } from "#/api/typesGenerated";
import { getWorkspaceQuotaQueryKey } from "./workspaceQuota";
import {
	autoCreateWorkspace,
	buildLogsKey,
	createWorkspace,
	invalidateWorkspaceListQueries,
	invalidateWorkspaceMutationQueries,
	workspacesKey,
	workspacesQueryKeyPrefix,
	workspaceUsage,
} from "./workspaces";

const createTestQueryClient = (): QueryClient =>
	new QueryClient({
		defaultOptions: {
			queries: {
				retry: false,
				gcTime: Number.POSITIVE_INFINITY,
				refetchOnWindowFocus: false,
				networkMode: "offlineFirst",
			},
		},
	});

const workspacesResponse = {
	workspaces: [],
	count: 0,
} satisfies WorkspacesResponse;

const seedWorkspaceFamilyQueries = (queryClient: QueryClient) => {
	const rawListKey = workspacesQueryKeyPrefix;
	const defaultListKey = workspacesKey({});
	const filteredListKey = workspacesKey({
		q: "owner:me organization:default",
		limit: 25,
		offset: 50,
	});
	const usageKey = workspaceUsage({
		usageApp: "reconnecting-pty",
		connectionStatus: "connected",
		workspaceId: "workspace-1",
		agentId: "agent-1",
	}).queryKey;
	const buildLogs = buildLogsKey("workspace-1");
	const workspacePermissionsKey = [
		"workspaces",
		"workspace-1",
		"permissions",
	] as const;
	const workspaceAgentCredentialsKey = [
		"workspaces",
		"workspace-1",
		"agents",
		"main",
		"credentials",
	] as const;
	const organizationWorkspacePermissionsKey = [
		"workspaces",
		["organization-1"],
		"permissions",
	] as const;

	queryClient.setQueryData(rawListKey, workspacesResponse);
	queryClient.setQueryData(defaultListKey, workspacesResponse);
	queryClient.setQueryData(filteredListKey, workspacesResponse);
	queryClient.setQueryData(usageKey, { tracked: true });
	queryClient.setQueryData(buildLogs, []);
	queryClient.setQueryData(workspacePermissionsKey, { read: true });
	queryClient.setQueryData(workspaceAgentCredentialsKey, { token: "secret" });
	queryClient.setQueryData(organizationWorkspacePermissionsKey, { read: true });

	return {
		listKeys: [rawListKey, defaultListKey, filteredListKey],
		nonListKeys: [
			usageKey,
			buildLogs,
			workspacePermissionsKey,
			workspaceAgentCredentialsKey,
			organizationWorkspacePermissionsKey,
		],
	};
};

describe("invalidateWorkspaceListQueries", () => {
	it("invalidates workspace list queries without touching side-effecting workspace-family queries", async () => {
		const queryClient = createTestQueryClient();
		const { listKeys, nonListKeys } = seedWorkspaceFamilyQueries(queryClient);

		await invalidateWorkspaceListQueries(queryClient);

		for (const key of listKeys) {
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${JSON.stringify(key)} should be invalidated`,
			).toBe(true);
		}
		for (const key of nonListKeys) {
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${JSON.stringify(key)} should NOT be invalidated`,
			).not.toBe(true);
		}
	});
});

describe("invalidateWorkspaceMutationQueries", () => {
	it("uses narrowed list invalidation and keeps workspace usage queries untouched", async () => {
		const queryClient = createTestQueryClient();
		const { listKeys, nonListKeys } = seedWorkspaceFamilyQueries(queryClient);
		const quotaKey = getWorkspaceQuotaQueryKey("default", "me");
		queryClient.setQueryData(quotaKey, { credits_consumed: 1, budget: 10 });

		await invalidateWorkspaceMutationQueries(queryClient, {
			organizationName: "default",
			username: "me",
		});

		for (const key of listKeys) {
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${JSON.stringify(key)} should be invalidated`,
			).toBe(true);
		}
		expect(queryClient.getQueryState(quotaKey)?.isInvalidated).toBe(true);
		for (const key of nonListKeys) {
			expect(
				queryClient.getQueryState(key)?.isInvalidated,
				`${JSON.stringify(key)} should NOT be invalidated`,
			).not.toBe(true);
		}
	});
});

describe("workspace creation mutations", () => {
	it("use narrowed list invalidation for manual workspace creation", async () => {
		const queryClient = createTestQueryClient();
		const { listKeys, nonListKeys } = seedWorkspaceFamilyQueries(queryClient);

		await createWorkspace(queryClient).onSuccess();

		for (const key of listKeys) {
			expect(queryClient.getQueryState(key)?.isInvalidated).toBe(true);
		}
		for (const key of nonListKeys) {
			expect(queryClient.getQueryState(key)?.isInvalidated).not.toBe(true);
		}
	});

	it("use narrowed list invalidation for auto workspace creation", async () => {
		const queryClient = createTestQueryClient();
		const { listKeys, nonListKeys } = seedWorkspaceFamilyQueries(queryClient);

		await autoCreateWorkspace(queryClient).onSuccess();

		for (const key of listKeys) {
			expect(queryClient.getQueryState(key)?.isInvalidated).toBe(true);
		}
		for (const key of nonListKeys) {
			expect(queryClient.getQueryState(key)?.isInvalidated).not.toBe(true);
		}
	});
});
