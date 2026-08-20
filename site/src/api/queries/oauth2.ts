import type { QueryClient } from "react-query";
import { API } from "#/api/api";
import type * as TypesGen from "#/api/typesGenerated";
import { useFilterParamsKey } from "#/components/Filter/Filter";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";
import { prepareQuery } from "#/utils/filters";

const oauth2ProviderKey = ["oauth2-provider"];
export const oauth2ProviderAppsKey = oauth2ProviderKey.concat("apps");
export const oauth2ProviderAppKey = (appId: string) =>
	oauth2ProviderAppsKey.concat(appId);
export const oauth2ProviderAppSecretsKey = (appId: string) =>
	oauth2ProviderAppKey(appId).concat("secrets");

const userAppsKey = (userId: string) => oauth2ProviderAppsKey.concat(userId);
export const oauth2ProviderSettingsKey = oauth2ProviderKey.concat("settings");

export const getGitHubDevice = () => {
	return {
		queryKey: ["oauth2-provider", "github", "device"],
		queryFn: () => API.getOAuth2GitHubDevice(),
	};
};

export const getGitHubDeviceFlowCallback = (code: string, state: string) => {
	return {
		queryKey: ["oauth2-provider", "github", "callback", code, state],
		queryFn: () => API.getOAuth2GitHubDeviceFlowCallback(code, state),
	};
};

export function oauth2AppsKey(req: TypesGen.OAuth2ProviderAppFilter) {
	return [...oauth2ProviderAppsKey, req] as const;
}

export function paginatedApps(
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<
	TypesGen.OAuth2ProviderAppsResponse,
	TypesGen.OAuth2ProviderAppFilter
> {
	return {
		searchParams,
		queryPayload: ({ limit, offset }) => {
			return {
				limit,
				offset,
				q: prepareQuery(searchParams.get(useFilterParamsKey) ?? ""),
			};
		},
		queryKey: ({ payload }) => oauth2AppsKey(payload),
		queryFn: ({ payload, signal }) =>
			API.getOAuth2ProviderApps(payload, signal),
	};
}

export const getApps = (userId?: string) => {
	return {
		queryKey: userId ? userAppsKey(userId) : oauth2ProviderAppsKey,
		queryFn: () => API.getOAuth2ProviderApps({ user_id: userId }),
	};
};

export const getApp = (id: string) => {
	return {
		queryKey: oauth2ProviderAppKey(id),
		queryFn: () => API.getOAuth2ProviderApp(id),
	};
};

export const postApp = (queryClient: QueryClient) => {
	return {
		mutationFn: API.postOAuth2ProviderApp,
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: oauth2ProviderAppsKey,
			});
		},
	};
};

export const putApp = (queryClient: QueryClient) => {
	return {
		mutationFn: ({
			id,
			req,
		}: {
			id: string;
			req: TypesGen.PutOAuth2ProviderAppRequest;
		}) => API.putOAuth2ProviderApp(id, req),
		onSuccess: async (app: TypesGen.OAuth2ProviderApp) => {
			queryClient.setQueryData(oauth2ProviderAppKey(app.id), app);
			await queryClient.invalidateQueries({
				queryKey: oauth2ProviderAppsKey,
			});
		},
	};
};

export const deleteApp = (queryClient: QueryClient) => {
	return {
		mutationFn: API.deleteOAuth2ProviderApp,
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: oauth2ProviderAppsKey,
			});
		},
	};
};

export const getAppSecrets = (id: string) => {
	return {
		queryKey: oauth2ProviderAppSecretsKey(id),
		queryFn: () => API.getOAuth2ProviderAppSecrets(id),
	};
};

export const postAppSecret = (queryClient: QueryClient) => {
	return {
		mutationFn: API.postOAuth2ProviderAppSecret,
		onSuccess: async (
			_: TypesGen.OAuth2ProviderAppSecretFull,
			appId: string,
		) => {
			await queryClient.invalidateQueries({
				queryKey: oauth2ProviderAppSecretsKey(appId),
			});
		},
	};
};

export const deleteAppSecret = (queryClient: QueryClient) => {
	return {
		mutationFn: ({ appId, secretId }: { appId: string; secretId: string }) =>
			API.deleteOAuth2ProviderAppSecret(appId, secretId),
		onSuccess: async (_: unknown, { appId }: { appId: string }) => {
			await queryClient.invalidateQueries({
				queryKey: oauth2ProviderAppSecretsKey(appId),
			});
		},
	};
};

export const revokeApp = (queryClient: QueryClient, userId: string) => {
	return {
		mutationFn: API.revokeOAuth2ProviderApp,
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: userAppsKey(userId),
			});
		},
	};
};

export const getSettings = () => {
	return {
		queryKey: oauth2ProviderSettingsKey,
		queryFn: () => API.getOAuth2ProviderSettings(),
	};
};

export const putSettings = (queryClient: QueryClient) => {
	return {
		mutationFn: API.putOAuth2ProviderSettings,
		// Seed from the response before invalidating. Invalidating resolves
		// whether or not the refetch succeeds, and a failed refetch keeps the
		// last successful data, which would render the pre-save value under an
		// error alert for a save that worked.
		onSuccess: async (settings: TypesGen.OAuth2ProviderSettings) => {
			queryClient.setQueryData(oauth2ProviderSettingsKey, settings);
			await queryClient.invalidateQueries({
				queryKey: oauth2ProviderSettingsKey,
			});
		},
	};
};
