import { UseQueryOptions } from "react-query";
import * as API from "api/api";
import { ProvisionerDaemon } from "api/typesGenerated";

export const provisionerDaemons = (
  orgId: string,
): UseQueryOptions<ProvisionerDaemon[]> => {
  return {
    queryKey: [orgId, "provisionerDaemons"],
    queryFn: () => API.getProvisionerDaemons(orgId),
  };
};
