import type {
  QueryClient,
  QueryKey,
  UseMutationOptions,
  UseQueryOptions,
} from "react-query";
import * as API from "api/api";
import type {
  AuthorizationRequest,
  GetUsersResponse,
  UpdateUserPasswordRequest,
  UpdateUserProfileRequest,
  UpdateUserAppearanceSettingsRequest,
  UsersRequest,
  User,
  GenerateAPIKeyResponse,
} from "api/typesGenerated";
import type { UsePaginatedQueryOptions } from "hooks/usePaginatedQuery";
import { prepareQuery } from "utils/filters";
import { getMetadataAsJSON } from "utils/metadata";
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
    cacheTime: 5 * 1000 * 60,
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
      await queryClient.invalidateQueries(["users"]);
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
      await queryClient.invalidateQueries(["users"]);
    },
  };
};

export const activateUser = (queryClient: QueryClient) => {
  return {
    mutationFn: API.activateUser,
    onSuccess: async () => {
      await queryClient.invalidateQueries(["users"]);
    },
  };
};

export const deleteUser = (queryClient: QueryClient) => {
  return {
    mutationFn: API.deleteUser,
    onSuccess: async () => {
      await queryClient.invalidateQueries(["users"]);
    },
  };
};

export const updateRoles = (queryClient: QueryClient) => {
  return {
    mutationFn: ({ userId, roles }: { userId: string; roles: string[] }) =>
      API.updateUserRoles(roles, userId),
    onSuccess: async () => {
      await queryClient.invalidateQueries(["users"]);
    },
  };
};

const initialUserData = getMetadataAsJSON<User>("user");

export const authMethods = () => {
  return {
    // Even the endpoint being /users/authmethods we don't want to revalidate it
    // when users change so its better add a unique query key
    queryKey: ["authMethods"],
    queryFn: API.getAuthMethods,
  };
};

const meKey = ["me"];

export const me = (): UseQueryOptions<User> & {
  queryKey: QueryKey;
} => {
  return cachedQuery({
    initialData: initialUserData,
    queryKey: meKey,
    queryFn: API.getAuthenticatedUser,
  });
};

export function apiKey(): UseQueryOptions<GenerateAPIKeyResponse> {
  return {
    queryKey: [...meKey, "apiKey"],
    queryFn: () => API.getApiKey(),
  };
}

export const hasFirstUser = (): UseQueryOptions<boolean> => {
  return cachedQuery({
    // This cannot be false otherwise it will not fetch!
    initialData: Boolean(initialUserData) || undefined,
    queryKey: ["hasFirstUser"],
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
      queryClient.setQueryData(["me"], data.user);
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

export const logout = (queryClient: QueryClient) => {
  return {
    mutationFn: API.logout,
    onSuccess: () => {
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

export const updateAppearanceSettings = (
  userId: string,
  queryClient: QueryClient,
): UseMutationOptions<
  User,
  unknown,
  UpdateUserAppearanceSettingsRequest,
  unknown
> => {
  return {
    mutationFn: (req) => API.updateAppearanceSettings(userId, req),
    onMutate: async (patch) => {
      // Mutate the `queryClient` optimistically to make the theme switcher
      // more responsive.
      const me: User | undefined = queryClient.getQueryData(meKey);
      if (userId === "me" && me) {
        queryClient.setQueryData(meKey, {
          ...me,
          theme_preference: patch.theme_preference,
        });
      }
    },
    onSuccess: async () => {
      // Could technically invalidate more, but we only ever care about the
      // `theme_preference` for the `me` query.
      if (userId === "me") {
        await queryClient.invalidateQueries(meKey);
      }
    },
  };
};
