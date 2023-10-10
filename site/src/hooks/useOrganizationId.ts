import { useAuth } from "components/AuthProvider/AuthProvider";
import { isAuthenticated } from "xServices/auth/authXService";

export const useOrganizationId = (): string => {
  const { actor } = useAuth();
  const [authState] = actor;
  const { data } = authState.context;

  if (isAuthenticated(data)) {
    return data.user.organization_ids[0];
  }

  throw new Error("User is not authenticated");
};
