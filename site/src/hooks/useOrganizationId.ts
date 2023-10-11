import { useMe } from "./useMe";

export const useOrganizationId = (): string => {
  const user = useMe();
  return user.organization_ids[0];
};
