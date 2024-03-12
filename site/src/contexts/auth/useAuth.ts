import { useContext } from "react";
import { AuthContext, type AuthContextValue } from "./AuthProvider";

export const useAuth = () => {
  const context = useContext(AuthContext);

  if (!context) {
    throw new Error("useAuth should be used inside of <AuthProvider />");
  }

  return context;
};



export const useAuthenticated = () => {
  const auth = useAuth()

  if(!auth.user) {
    throw new Error("User is not authenticated.")
  }

  // We can do some TS magic here but I would rather to be explicit on what
  // values are not undefined when authenticated
  return auth as AuthContextValue & {
    user: Exclude<AuthContextValue['user'], undefined>,
    permissions: Exclude<AuthContextValue['permissions'], undefined>,
    orgId: Exclude<AuthContextValue['orgId'], undefined>,
  }
}

