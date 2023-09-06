import { useActor, useInterpret } from "@xstate/react";
import { createContext, FC, PropsWithChildren, useContext } from "react";
import { authMachine } from "xServices/auth/authXService";
import { ActorRefFrom } from "xstate";

interface AuthContextValue {
  authService: ActorRefFrom<typeof authMachine>;
}

const AuthContext = createContext<AuthContextValue | undefined>(undefined);

export const AuthProvider: FC<PropsWithChildren> = ({ children }) => {
  const authService = useInterpret(authMachine);

  return (
    <AuthContext.Provider value={{ authService }}>
      {children}
    </AuthContext.Provider>
  );
};

type UseAuthReturnType = ReturnType<
  typeof useActor<AuthContextValue["authService"]>
>;

export const useAuth = (): UseAuthReturnType => {
  const context = useContext(AuthContext);

  if (!context) {
    throw new Error("useAuth should be used inside of <AuthProvider />");
  }

  const auth = useActor(context.authService);

  return auth;
};
