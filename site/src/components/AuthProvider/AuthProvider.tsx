import { useActor, useInterpret } from "@xstate/react";
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
import {
  Permissions,
  authMachine,
  isAuthenticated,
} from "xServices/auth/authXService";
import { ActorRefFrom } from "xstate";

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
  authService: ActorRefFrom<typeof authMachine>;
};

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export const AuthProvider: FC<PropsWithChildren> = ({ children }) => {
  const authService = useInterpret(authMachine);
  const [authState, authSend] = useActor(authService);

  const isSignedOut = authState.matches("signedOut");
  const isSigningOut = authState.matches("signingOut");
  const isLoading = authState.matches("loadingInitialAuthData");
  const isConfiguringTheFirstUser = authState.matches(
    "configuringTheFirstUser",
  );
  const isSignedIn = authState.matches("signedIn");
  const isSigningIn = authState.matches("signingIn");
  const isUpdatingProfile = authState.matches(
    "signedIn.profile.updatingProfile",
  );

  const signOut = useCallback(() => {
    authSend("SIGN_OUT");
  }, [authSend]);

  const signIn = useCallback(
    (email: string, password: string) => {
      authSend({ type: "SIGN_IN", email, password });
    },
    [authSend],
  );

  const updateProfile = useCallback(
    (data: UpdateUserProfileRequest) => {
      authSend({ type: "UPDATE_PROFILE", data });
    },
    [authSend],
  );

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
        authService,
        signOut,
        signIn,
        updateProfile,
        user: isAuthenticated(authState.context.data)
          ? authState.context.data.user
          : undefined,
        permissions: isAuthenticated(authState.context.data)
          ? authState.context.data.permissions
          : undefined,
        authMethods: !isAuthenticated(authState.context.data)
          ? authState.context.data?.authMethods
          : undefined,
        signInError: authState.context.error,
        updateProfileError: authState.context.updateProfileError,
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

  return {
    ...context,
    actor: useActor(context.authService),
  };
};
