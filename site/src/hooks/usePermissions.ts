import { useAuth } from "contexts/AuthProvider/AuthProvider";
import type { Permissions } from "contexts/AuthProvider/permissions";

export const usePermissions = (): Permissions => {
  const { permissions } = useAuth();

  if (!permissions) {
    throw new Error("User is not authenticated.");
  }

  return permissions;
};
