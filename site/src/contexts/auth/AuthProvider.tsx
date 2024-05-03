import {
  type FC,
  type PropsWithChildren,
  createContext,
  useCallback,
  useContext,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { isApiError } from "api/errors";
import { checkAuthorization } from "api/queries/authCheck";
import {
  hasFirstUser,
  login,
  logout,
  me,
  updateProfile as updateProfileOptions,
} from "api/queries/users";
import type { UpdateUserProfileRequest, User } from "api/typesGenerated";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { useEmbeddedMetadata } from "hooks/useEmbeddedMetadata";
import { permissionsToCheck, type Permissions } from "./permissions";

export type AuthContextValue = {
  isLoading: boolean;
  isSignedOut: boolean;
  isSigningOut: boolean;
  isConfiguringTheFirstUser: boolean;
  isSignedIn: boolean;
  isSigningIn: boolean;
  isUpdatingProfile: boolean;
  user: User | undefined;
  permissions: Permissions | undefined;
  organizationId: string | undefined;
  signInError: unknown;
  updateProfileError: unknown;
  signOut: () => void;
  signIn: (email: string, password: string) => Promise<void>;
  updateProfile: (data: UpdateUserProfileRequest) => void;
};

export const AuthContext = createContext<AuthContextValue | undefined>(
  undefined,
);

export const AuthProvider: FC<PropsWithChildren> = ({ children }) => {
  const { metadata } = useEmbeddedMetadata();
  const userMetadataState = metadata.user;

  const meOptions = me(userMetadataState);
  const userQuery = useQuery(meOptions);
  const hasFirstUserQuery = useQuery(hasFirstUser(userMetadataState));

  const permissionsQuery = useQuery({
    ...checkAuthorization({ checks: permissionsToCheck }),
    enabled: userQuery.data !== undefined,
  });

  const queryClient = useQueryClient();
  const loginMutation = useMutation(
    login({ checks: permissionsToCheck }, queryClient),
  );

  const logoutMutation = useMutation(logout(queryClient));
  const updateProfileMutation = useMutation({
    ...updateProfileOptions("me"),
    onSuccess: (user) => {
      queryClient.setQueryData(meOptions.queryKey, user);
      displaySuccess("Updated settings.");
    },
  });

  const isSignedOut =
    userQuery.isError &&
    isApiError(userQuery.error) &&
    userQuery.error.response.status === 401;
  const isSigningOut = logoutMutation.isLoading;
  const isLoading =
    userQuery.isLoading ||
    hasFirstUserQuery.isLoading ||
    (userQuery.isSuccess && permissionsQuery.isLoading);
  const isConfiguringTheFirstUser =
    !hasFirstUserQuery.isLoading && !hasFirstUserQuery.data;
  const isSignedIn = userQuery.isSuccess && userQuery.data !== undefined;
  const isSigningIn = loginMutation.isLoading;
  const isUpdatingProfile = updateProfileMutation.isLoading;

  const signOut = useCallback(() => {
    logoutMutation.mutate();
  }, [logoutMutation]);

  const signIn = useCallback(
    async (email: string, password: string) => {
      await loginMutation.mutateAsync({ email, password });
    },
    [loginMutation],
  );

  const updateProfile = useCallback(
    (req: UpdateUserProfileRequest) => {
      updateProfileMutation.mutate(req);
    },
    [updateProfileMutation],
  );

  return (
    <AuthContext.Provider
      value={{
        isLoading,
        isSignedOut,
        isSigningOut,
        isConfiguringTheFirstUser,
        isSignedIn,
        isSigningIn,
        isUpdatingProfile,
        signOut,
        signIn,
        updateProfile,
        user: userQuery.data,
        permissions: permissionsQuery.data as Permissions | undefined,
        signInError: loginMutation.error,
        updateProfileError: updateProfileMutation.error,
        organizationId: userQuery.data?.organization_ids[0],
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

export const useAuthContext = () => {
  const context = useContext(AuthContext);

  if (!context) {
    throw new Error("useAuth should be used inside of <AuthProvider />");
  }

  return context;
};
