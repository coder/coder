import {
  deleteWorkspaceAgentSharedPort,
  getWorkspaceAgentSharedPorts,
  upsertWorkspaceAgentSharedPort,
} from "api/api";
import {
  DeleteWorkspaceAgentPortShareRequest,
  UpsertWorkspaceAgentPortShareRequest,
} from "api/typesGenerated";

export const workspacePortShares = (workspaceId: string) => {
  return {
    queryKey: ["sharedPorts", workspaceId],
    queryFn: () => getWorkspaceAgentSharedPorts(workspaceId),
  };
};

export const upsertWorkspacePortShare = (workspaceId: string) => {
  return {
    mutationFn: async (options: UpsertWorkspaceAgentPortShareRequest) => {
      await upsertWorkspaceAgentSharedPort(workspaceId, options);
    },
  };
};

export const deleteWorkspacePortShare = (workspaceId: string) => {
  return {
    mutationFn: async (options: DeleteWorkspaceAgentPortShareRequest) => {
      await deleteWorkspaceAgentSharedPort(workspaceId, options);
    },
  };
};
