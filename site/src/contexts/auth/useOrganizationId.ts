import { useAuthenticated } from "./useAuth";

export const useOrganizationId = (): string => {
  const { user: me } = useAuthenticated();

  if (me.organization_ids.length < 1) {
    throw new Error("User is not a member of any organizations");
  }

  return me.organization_ids[0];
};
