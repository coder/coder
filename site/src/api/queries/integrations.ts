import type { GetJFrogXRayScanParams } from "api/api";
import * as API from "api/api";
import type { WorkspaceAgentStatus } from "api/typesGenerated";

export const xrayScan = (
  params: GetJFrogXRayScanParams,
  status: WorkspaceAgentStatus,
) => {
  return {
    // Reload the xray results whenever the status changes
    queryKey: ["xray", params, status],
    queryFn: () => API.getJFrogXRayScan(params),
  };
};
