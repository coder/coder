import { useActor, useInterpret } from "@xstate/react";
import { UpdateUserProfileRequest } from "api/typesGenerated";
import {
  createContext,
  FC,
  PropsWithChildren,
  useCallback,
  useContext,
} from "react";
import { authMachine } from "xServices/auth/authXService";
import { ActorRefFrom } from "xstate";

type AuthContextValue = {
  signOut: () => void;
  signIn: (email: string, password: string) => void;
  updateProfile: (data: UpdateUserProfileRequest) => void;
  authService: ActorRefFrom<typeof authMachine>;
};

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export const AuthProvider: FC<PropsWithChildren> = ({ children }) => {
  const authService = useInterpret(authMachine);

  const signOut = useCallback(() => {
    authService.send("SIGN_OUT");
  }, [authService]);

  const signIn = useCallback(
    (email: string, password: string) => {
      authService.send({ type: "SIGN_IN", email, password });
    },
    [authService],
  );

  const updateProfile = useCallback(
    (data: UpdateUserProfileRequest) => {
      authService.send({ type: "UPDATE_PROFILE", data });
    },
    [authService],
  );

  return (
    <AuthContext.Provider
      value={{ authService, signOut, signIn, updateProfile }}
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
