import * as API from "api/api";

export const groups = (organizationId: string) => {
  return {
    queryKey: ["groups"],
    queryFn: () => API.getGroups(organizationId),
  };
};
