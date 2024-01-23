import { useMe } from "./useMe";

export const useOrganizationId = (): string => {
  const me = useMe();

  if (me.organization_ids.length < 1) {
    throw new Error("User is not a member of any organizations");
  }

  return me.organization_ids[0];
};
