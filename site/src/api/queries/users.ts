import { QueryClient, QueryOptions } from "react-query";
import * as API from "api/api";
import {
  AuthorizationRequest,
  GetUsersResponse,
  UpdateUserPasswordRequest,
  UpdateUserProfileRequest,
  User,
  UsersRequest,
} from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";
import { getAuthorizationKey } from "./authCheck";

export const users = (req: UsersRequest): QueryOptions<GetUsersResponse> => {
  return {
    queryKey: ["users", req],
    queryFn: ({ signal }) => API.getUsers(req, signal),
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

export const authMethods = () => {
  return {
    // Even the endpoint being /users/authmethods we don't want to revalidate it
    // when users change so its better add a unique query key
    queryKey: ["authMethods"],
    queryFn: API.getAuthMethods,
  };
};

export const me = () => {
  return {
    queryKey: ["me"],
    queryFn: async () =>
      getMetadataAsJSON<User>("user") ?? API.getAuthenticatedUser(),
  };
};

export const hasFirstUser = () => {
  return {
    queryKey: ["hasFirstUser"],
    queryFn: API.hasFirstUser,
  };
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

export const updateProfile = () => {
  return {
    mutationFn: ({
      userId,
      req,
    }: {
      userId: string;
      req: UpdateUserProfileRequest;
    }) => API.updateProfile(userId, req),
  };
};
