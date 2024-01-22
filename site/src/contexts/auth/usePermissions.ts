import { useAuth } from "./useAuth";
import type { Permissions } from "./permissions";

export const usePermissions = (): Permissions => {
  const { permissions } = useAuth();

  if (!permissions) {
    throw new Error("User is not authenticated.");
  }

  return permissions;
};
