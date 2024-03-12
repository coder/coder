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

  return auth as  & {
    user: Exclude<AuthContextValue['user'], undefined>,
    permissions: Exclude<AuthContextValue['permissions'], undefined>,
  }
}

