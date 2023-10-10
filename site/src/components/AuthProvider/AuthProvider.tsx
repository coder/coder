import { checkAuthorization } from "api/queries/authCheck";
import {
  authMethods,
  hasFirstUser,
  login,
  logout,
  me,
  updateProfile as updateProfileOptions,
} from "api/queries/users";
import {
  AuthMethods,
  UpdateUserProfileRequest,
  User,
} from "api/typesGenerated";
import { createContext, FC, PropsWithChildren, useContext } from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { permissionsToCheck, Permissions } from "./permissions";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";

type AuthContextValue = {
  isSignedOut: boolean;
  isLoading: boolean;
  isSigningOut: boolean;
  isConfiguringTheFirstUser: boolean;
  isSignedIn: boolean;
  isSigningIn: boolean;
  isUpdatingProfile: boolean;
  user: User | undefined;
  permissions: Permissions | undefined;
  authMethods: AuthMethods | undefined;
  signInError: unknown;
  updateProfileError: unknown;
  signOut: () => void;
  signIn: (email: string, password: string) => void;
  updateProfile: (data: UpdateUserProfileRequest) => void;
};

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export const AuthProvider: FC<PropsWithChildren> = ({ children }) => {
  const meOptions = me();
  const userQuery = useQuery(meOptions);
  const authMethodsQuery = useQuery(authMethods());
  const hasFirstUserQuery = useQuery(hasFirstUser());
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
    ...updateProfileOptions(),
    onSuccess: (user) => {
      queryClient.setQueryData(meOptions.queryKey, user);
      displaySuccess("Updated settings.");
    },
  });

  const isSignedOut = userQuery.isSuccess && !userQuery.data;
  const isSigningOut = logoutMutation.isLoading;
  const isLoading =
    authMethodsQuery.isLoading ||
    userQuery.isLoading ||
    permissionsQuery.isLoading ||
    hasFirstUserQuery.isLoading;
  const isConfiguringTheFirstUser = !hasFirstUserQuery.data;
  const isSignedIn = userQuery.isSuccess && userQuery.data !== undefined;
  const isSigningIn = loginMutation.isLoading;
  const isUpdatingProfile = updateProfileMutation.isLoading;

  const signOut = logoutMutation.mutate;

  const signIn = (email: string, password: string) => {
    loginMutation.mutate({ email, password });
  };

  const updateProfile = (req: UpdateUserProfileRequest) => {
    updateProfileMutation.mutate({ userId: userQuery.data!.id, req });
  };

  if (isLoading) {
    return <FullScreenLoader />;
  }

  return (
    <AuthContext.Provider
      value={{
        isSignedOut,
        isSigningOut,
        isLoading,
        isConfiguringTheFirstUser,
        isSignedIn,
        isSigningIn,
        isUpdatingProfile,
        signOut,
        signIn,
        updateProfile,
        user: userQuery.data,
        permissions: permissionsQuery.data as Permissions | undefined,
        authMethods: authMethodsQuery.data,
        signInError: loginMutation.error,
        updateProfileError: updateProfileMutation.error,
      }}
    >
      {children}
    </AuthContext.Provider>
  );
};

export const useAuth = () => {
  const context = useContext(AuthContext);

  if (!context) {
    throw new Error("useAuth should be used inside of <AuthProvider />");
  }

  return context;
};
