import { client } from "api/api";
import type {
  DeleteWorkspaceAgentPortShareRequest,
  UpsertWorkspaceAgentPortShareRequest,
} from "api/typesGenerated";

export const workspacePortShares = (workspaceId: string) => {
  return {
    queryKey: ["sharedPorts", workspaceId],
    queryFn: () => client.api.getWorkspaceAgentSharedPorts(workspaceId),
  };
};

export const upsertWorkspacePortShare = (workspaceId: string) => {
  return {
    mutationFn: async (options: UpsertWorkspaceAgentPortShareRequest) => {
      await client.api.upsertWorkspaceAgentSharedPort(workspaceId, options);
    },
  };
};

export const deleteWorkspacePortShare = (workspaceId: string) => {
  return {
    mutationFn: async (options: DeleteWorkspaceAgentPortShareRequest) => {
      await client.api.deleteWorkspaceAgentSharedPort(workspaceId, options);
    },
  };
};
