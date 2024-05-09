import { API } from "api/api";
import type {
  DeleteWorkspaceAgentPortShareRequest,
  UpsertWorkspaceAgentPortShareRequest,
} from "api/typesGenerated";

export const workspacePortShares = (workspaceId: string) => {
  return {
    queryKey: ["sharedPorts", workspaceId],
    queryFn: () => API.getWorkspaceAgentSharedPorts(workspaceId),
  };
};

export const upsertWorkspacePortShare = (workspaceId: string) => {
  return {
    mutationFn: async (options: UpsertWorkspaceAgentPortShareRequest) => {
      await API.upsertWorkspaceAgentSharedPort(workspaceId, options);
    },
  };
};

export const deleteWorkspacePortShare = (workspaceId: string) => {
  return {
    mutationFn: async (options: DeleteWorkspaceAgentPortShareRequest) => {
      await API.deleteWorkspaceAgentSharedPort(workspaceId, options);
    },
  };
};
