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
import {
  createContext,
  FC,
  PropsWithChildren,
  useCallback,
  useContext,
} from "react";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { permissionsToCheck, Permissions } from "./permissions";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { FullScreenLoader } from "components/Loader/FullScreenLoader";
import { isApiError } from "api/errors";

type AuthContextValue = {
  isSignedOut: boolean;
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
  signIn: (email: string, password: string) => Promise<void>;
  updateProfile: (data: UpdateUserProfileRequest) => void;
};

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export const AuthProvider: FC<PropsWithChildren> = ({ children }) => {
  const queryClient = useQueryClient();
  const meOptions = me();

  const userQuery = useQuery(meOptions);
  const authMethodsQuery = useQuery(authMethods());
  const hasFirstUserQuery = useQuery(hasFirstUser());
  const permissionsQuery = useQuery({
    ...checkAuthorization({ checks: permissionsToCheck }),
    enabled: userQuery.data !== undefined,
  });

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

  const isSignedOut =
    userQuery.isError &&
    isApiError(userQuery.error) &&
    userQuery.error.response.status === 401;
  const isSigningOut = logoutMutation.isLoading;
  const isLoading =
    authMethodsQuery.isLoading ||
    userQuery.isLoading ||
    hasFirstUserQuery.isLoading ||
    (userQuery.isSuccess && permissionsQuery.isLoading);
  const isConfiguringTheFirstUser = !hasFirstUserQuery.data;
  const isSignedIn = userQuery.isSuccess && userQuery.data !== undefined;
  const isSigningIn = loginMutation.isLoading;
  const isUpdatingProfile = updateProfileMutation.isLoading;

  const signOut = useCallback(() => {
    logoutMutation.mutate();
  }, [logoutMutation]);

  const signIn = async (email: string, password: string) => {
    await loginMutation.mutateAsync({ email, password });
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
