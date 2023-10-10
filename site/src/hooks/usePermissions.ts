import { useAuth } from "components/AuthProvider/AuthProvider";
import { isAuthenticated, Permissions } from "xServices/auth/authXService";

export const usePermissions = (): Permissions => {
  const { actor } = useAuth();
  const [authState] = actor;
  const { data } = authState.context;

  if (isAuthenticated(data)) {
    return data.permissions;
  }

  throw new Error("User is not authenticated.");
};
