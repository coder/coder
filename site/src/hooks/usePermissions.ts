import { useAuth } from "components/AuthProvider/AuthProvider";
import { Permissions } from "xServices/auth/authXService";

export const usePermissions = (): Permissions => {
  const { permissions } = useAuth();

  if (!permissions) {
    throw new Error("User is not authenticated.");
  }

  return permissions;
};
