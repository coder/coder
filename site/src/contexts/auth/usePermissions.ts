import type { Permissions } from "./permissions";
import { useAuth } from "./useAuth";

export const usePermissions = (): Permissions => {
  const { permissions } = useAuth();

  if (!permissions) {
    throw new Error("User is not authenticated.");
  }

  return permissions;
};
