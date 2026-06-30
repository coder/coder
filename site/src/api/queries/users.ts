import type {
	MutationOptions,
	QueryClient,
	UseMutationOptions,
	UseQueryOptions,
} from "react-query";
import { API, type UserAISpend } from "#/api/api";
import { isApiError } from "#/api/errors";
import type {
	AuthorizationRequest,
	GenerateAPIKeyResponse,
	GetUsersResponse,
	MinimalUser,
	RequestOneTimePasscodeRequest,
	UpdateUserAppearanceSettingsRequest,
	UpdateUserPasswordRequest,
	UpdateUserPreferenceSettingsRequest,
	UpdateUserProfileRequest,
	UpsertUserAIBudgetOverrideRequest,
	User,
	UserAIBudgetOverride,
	UserAppearanceSettings,
	UserPreferenceSettings,
	UsersRequest,
} from "#/api/typesGenerated";
import {
	defaultMetadataManager,
	type MetadataState,
} from "#/hooks/useEmbeddedMetadata";
import type { UsePaginatedQueryOptions } from "#/hooks/usePaginatedQuery";
import { prepareQuery } from "#/utils/filters";
import { getAuthorizationKey } from "./authCheck";
import { cachedQuery } from "./util";

export function usersKey(req: UsersRequest) {
	return ["users", req] as const;
}

export function paginatedUsers(
	searchParams: URLSearchParams,
): UsePaginatedQueryOptions<GetUsersResponse, UsersRequest> {
	return {
		searchParams,
		queryPayload: ({ limit, offset }) => {
			return {
				limit,
				offset,
				q: prepareQuery(searchParams.get("filter") ?? ""),
			};
		},

		queryKey: ({ payload }) => usersKey(payload),
		queryFn: ({ payload, signal }) => API.getUsers(payload, signal),
	};
}

export const users = (req: UsersRequest): UseQueryOptions<GetUsersResponse> => {
	return {
		queryKey: usersKey(req),
		queryFn: ({ signal }) => API.getUsers(req, signal),
		gcTime: 5 * 1000 * 60,
	};
};

export const workspaceAvailableUsers = (
	organizationId: string,
	req: UsersRequest,
): UseQueryOptions<MinimalUser[]> => {
	return {
		queryKey: ["workspaceAvailableUsers", organizationId, req],
		queryFn: ({ signal }) =>
			API.getWorkspaceAvailableUsers(organizationId, req, signal),
		gcTime: 5 * 1000 * 60,
	};
};

export const updatePassword = () => {
	return {
		mutationFn: ({
			userId,
			...request
		}: UpdateUserPasswordRequest & { userId: string }) =>
			API.updateUserPassword(userId, request),
	};
};

export const createUser = (queryClient: QueryClient) => {
	return {
		mutationFn: API.createUser,
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["users"] });
		},
	};
};

export const createFirstUser = () => {
	return {
		mutationFn: API.createFirstUser,
	};
};

export const suspendUser = (queryClient: QueryClient) => {
	return {
		mutationFn: API.suspendUser,
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["users"] });
		},
	};
};

export const activateUser = (queryClient: QueryClient) => {
	return {
		mutationFn: API.activateUser,
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["users"] });
		},
	};
};

export const deleteUser = (queryClient: QueryClient) => {
	return {
		mutationFn: API.deleteUser,
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["users"] });
		},
	};
};

export const updateRoles = (queryClient: QueryClient) => {
	return {
		mutationFn: ({ userId, roles }: { userId: string; roles: string[] }) =>
			API.updateUserRoles(roles, userId),
		onSuccess: async () => {
			await queryClient.invalidateQueries({ queryKey: ["users"] });
		},
	};
};

export const authMethodsQueryKey = ["authMethods"];

export const authMethods = () => {
	return {
		// Even the endpoint being /users/authmethods we don't want to revalidate it
		// when users change so its better add a unique query key
		queryKey: authMethodsQueryKey,
		queryFn: API.getAuthMethods,
	};
};

export const meKey = ["me"];

export const me = (metadata: MetadataState<User>) => {
	return cachedQuery({
		metadata,
		queryKey: meKey,
		queryFn: API.getAuthenticatedUser,
	});
};

export const meAISpendKey = [...meKey, "aiSpend"] as const;

export const meAISpend = (): UseQueryOptions<UserAISpend> => {
	return {
		queryKey: meAISpendKey,
		queryFn: () => API.getUserAISpend(),
		// Polled so the avatar border reflects spend without opening the dropdown.
		refetchInterval: 60_000,
	};
};

const userKey = (usernameOrId: string) => ["user", usernameOrId];

export const user = (usernameOrId: string) => {
	return {
		queryKey: userKey(usernameOrId),
		queryFn: () => API.getUser(usernameOrId),
	};
};

export const getUserAIBudgetOverrideQueryKey = (userId: string) => [
	"user",
	userId,
	"aiBudgetOverride",
];

export const userAIBudgetOverride = (
	userId: string,
): UseQueryOptions<UserAIBudgetOverride | null> => {
	return {
		queryKey: getUserAIBudgetOverrideQueryKey(userId),
		queryFn: async () => {
			try {
				return await API.getUserAIBudgetOverride(userId);
			} catch (error) {
				if (isApiError(error) && error.response.status === 404) {
					return null;
				}

				throw error;
			}
		},
	};
};

export const saveUserAIBudgetOverride = (
	queryClient: QueryClient,
	userId: string,
) => {
	return {
		mutationFn: (request: UpsertUserAIBudgetOverrideRequest) =>
			API.upsertUserAIBudgetOverride(userId, request),
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: getUserAIBudgetOverrideQueryKey(userId),
			});
		},
	};
};

export const deleteUserAIBudgetOverride = (
	queryClient: QueryClient,
	userId: string,
) => {
	return {
		mutationFn: () => API.deleteUserAIBudgetOverride(userId),
		onSuccess: async () => {
			await queryClient.invalidateQueries({
				queryKey: getUserAIBudgetOverrideQueryKey(userId),
			});
		},
	};
};

export function apiKey(): UseQueryOptions<GenerateAPIKeyResponse> {
	return {
		queryKey: [...meKey, "apiKey"],
		queryFn: () => API.getApiKey(),
	};
}

export const hasFirstUserKey = ["hasFirstUser"];

export const hasFirstUser = (userMetadata: MetadataState<User>) => {
	return cachedQuery({
		metadata: userMetadata,
		queryKey: hasFirstUserKey,
		queryFn: API.hasFirstUser,
	});
};

export const login = (
	authorization: AuthorizationRequest,
	queryClient: QueryClient,
) => {
	return {
		mutationFn: async (credentials: { email: string; password: string }) =>
			loginFn({ ...credentials, authorization }),
		onSuccess: async (data: Awaited<ReturnType<typeof loginFn>>) => {
			queryClient.setQueryData(meKey, data.user);
			queryClient.setQueryData(
				getAuthorizationKey(authorization),
				data.permissions,
			);
		},
	};
};

const loginFn = async ({
	email,
	password,
	authorization,
}: {
	email: string;
	password: string;
	authorization: AuthorizationRequest;
}) => {
	await API.login(email, password);
	const [user, permissions] = await Promise.all([
		API.getAuthenticatedUser(),
		API.checkAuthorization(authorization),
	]);
	return {
		user,
		permissions,
	};
};

export const logout = (queryClient: QueryClient): MutationOptions => {
	return {
		mutationFn: API.logout,
		// We're doing this cleanup in `onSettled` instead of `onSuccess` because in the case where an oAuth refresh token has expired this endpoint will return a 401 instead of 200.
		onSettled: (_, error) => {
			if (error) {
				console.error(error);
			}

			/**
			 * 2024-05-02 - If we persist any form of user data after the user logs
			 * out, that will continue to seed the React Query cache, creating
			 * "impossible" states where we'll have data we're not supposed to have.
			 *
			 * This has caused issues where logging out will instantly throw a
			 * completely uncaught runtime rendering error. Worse yet, the error only
			 * exists when serving the site from the Go backend (the JS environment
			 * has zero issues because it doesn't have access to the metadata). These
			 * errors can only be caught with E2E tests.
			 *
			 * Deleting the user data will mean that all future requests have to take
			 * a full roundtrip, but this still felt like the best way to ensure that
			 * manually logging out doesn't blow the entire app up.
			 *
			 * 2025-08-20 - Since this endpoint is for performing a post logout clean up
			 * on the backend we should move this local clean up outside of the mutation
			 * so that it can be explicitly performed even in cases where we don't want
			 * run the clean up (e.g. when a user is unauthorized). Unfortunately our
			 * auth logic is too tangled up with some obscured React Query behaviors to
			 * be able to move right now. After `AuthProvider.tsx` is refactored this
			 * should be moved.
			 */
			defaultMetadataManager.clearMetadataByKey("user");
			queryClient.removeQueries();
		},
	};
};

export const updateProfile = (userId: string) => {
	return {
		mutationFn: (req: UpdateUserProfileRequest) =>
			API.updateProfile(userId, req),
	};
};

export const myAppearanceKey = ["me", "appearance"] as const;

type AppearanceMutationContext = {
	previousAppearanceSettings: UserAppearanceSettings | undefined;
};

export const appearanceSettings = (
	metadata: MetadataState<UserAppearanceSettings>,
) => {
	return cachedQuery({
		metadata,
		queryKey: myAppearanceKey,
		queryFn: API.getAppearanceSettings,
	});
};

export const updateAppearanceSettings = (
	queryClient: QueryClient,
): UseMutationOptions<
	UserAppearanceSettings,
	unknown,
	UpdateUserAppearanceSettingsRequest,
	AppearanceMutationContext
> => {
	return {
		mutationFn: (req) => API.updateAppearanceSettings(req),
		onMutate: async (patch) => {
			await queryClient.cancelQueries({ queryKey: myAppearanceKey });
			const previousAppearanceSettings =
				queryClient.getQueryData<UserAppearanceSettings>(myAppearanceKey);

			// Mutate the `queryClient` optimistically to make the theme switcher
			// more responsive.
			queryClient.setQueryData<UserAppearanceSettings>(myAppearanceKey, {
				theme_preference: patch.theme_preference,
				theme_mode: patch.theme_mode,
				theme_light: patch.theme_light,
				theme_dark: patch.theme_dark,
				terminal_font: patch.terminal_font,
			});
			return { previousAppearanceSettings };
		},
		onError: (_error, _patch, context) => {
			if (context?.previousAppearanceSettings) {
				queryClient.setQueryData<UserAppearanceSettings>(
					myAppearanceKey,
					context.previousAppearanceSettings,
				);
				return;
			}
			queryClient.removeQueries({ queryKey: myAppearanceKey, exact: true });
		},
		onSuccess: (settings, patch) => {
			queryClient.setQueryData<UserAppearanceSettings>(myAppearanceKey, {
				...patch,
				...settings,
			});
		},
	};
};

const myPreferencesKey = ["me", "preferences"];

export const preferenceSettings =
	(): UseQueryOptions<UserPreferenceSettings> => {
		return {
			queryKey: myPreferencesKey,
			queryFn: () => API.getUserPreferenceSettings(),
		};
	};

export const updatePreferenceSettings = (
	queryClient: QueryClient,
): UseMutationOptions<
	UserPreferenceSettings,
	unknown,
	UpdateUserPreferenceSettingsRequest,
	unknown
> => {
	return {
		mutationFn: (req) => API.updateUserPreferenceSettings(req),
		onSuccess: async () =>
			await queryClient.invalidateQueries({
				queryKey: myPreferencesKey,
			}),
	};
};

export const requestOneTimePassword = () => {
	return {
		mutationFn: (req: RequestOneTimePasscodeRequest) =>
			API.requestOneTimePassword(req),
	};
};

export const changePasswordWithOTP = () => {
	return {
		mutationFn: API.changePasswordWithOTP,
	};
};
