import * as API from "api/api";
import { QueryClient, type QueryOptions } from "react-query";
import { putWorkspaceExtension } from "api/api";
import dayjs from "dayjs";
import { getDeadline, getMaxDeadline, getMinDeadline } from "utils/schedule";
import {
  type WorkspaceBuildParameter,
  type Workspace,
  type CreateWorkspaceRequest,
  type WorkspacesResponse,
  type WorkspacesRequest,
} from "api/typesGenerated";

export const workspaceByOwnerAndNameKey = (owner: string, name: string) => [
  "workspace",
  owner,
  name,
  "settings",
];

export const workspaceByOwnerAndName = (
  owner: string,
  name: string,
): QueryOptions<Workspace> => {
  return {
    queryKey: workspaceByOwnerAndNameKey(owner, name),
    queryFn: () => API.getWorkspaceByOwnerAndName(owner, name),
  };
};

type AutoCreateWorkspaceOptions = {
  templateName: string;
  versionId?: string;
  organizationId: string;
  defaultBuildParameters?: WorkspaceBuildParameter[];
  defaultName: string;
};

type CreateWorkspaceMutationVariables = CreateWorkspaceRequest & {
  userId: string;
  organizationId: string;
};

export const createWorkspace = (queryClient: QueryClient) => {
  return {
    mutationFn: async (variables: CreateWorkspaceMutationVariables) => {
      const { userId, organizationId, ...req } = variables;
      return API.createWorkspace(organizationId, userId, req);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries(["workspaces"]);
    },
  };
};

export const autoCreateWorkspace = (queryClient: QueryClient) => {
  return {
    mutationFn: async ({
      templateName,
      versionId,
      organizationId,
      defaultBuildParameters,
      defaultName,
    }: AutoCreateWorkspaceOptions) => {
      let templateVersionParameters;

      if (versionId) {
        templateVersionParameters = { template_version_id: versionId };
      } else {
        const template = await API.getTemplateByName(
          organizationId,
          templateName,
        );
        templateVersionParameters = { template_id: template.id };
      }

      return API.createWorkspace(organizationId, "me", {
        ...templateVersionParameters,
        name: defaultName,
        rich_parameter_values: defaultBuildParameters,
      });
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries(["workspaces"]);
    },
  };
};

export function workspacesKey(config: WorkspacesRequest = {}) {
  const { q, limit } = config;
  return ["workspaces", { q, limit }] as const;
}

export function workspaces(config: WorkspacesRequest = {}) {
  // Duplicates some of the work from workspacesKey, but that felt better than
  // letting invisible properties sneak into the query logic
  const { q, limit } = config;

  return {
    queryKey: workspacesKey(config),
    queryFn: () => API.getWorkspaces({ q, limit }),
  } as const satisfies QueryOptions<WorkspacesResponse>;
}

export const decreaseDeadline = (workspace: Workspace) => {
  return {
    mutationFn: (hours: number) => {
      const proposedDeadline = getDeadline(workspace).subtract(hours, "hours");
      const newDeadline = dayjs.max(proposedDeadline, getMinDeadline());
      return putWorkspaceExtension(workspace.id, newDeadline);
    },
  };
};

export const increaseDeadline = (workspace: Workspace) => {
  return {
    mutationFn: (hours: number) => {
      const proposedDeadline = getDeadline(workspace).add(hours, "hours");
      const newDeadline = dayjs.min(
        proposedDeadline,
        getMaxDeadline(workspace),
      );
      return putWorkspaceExtension(workspace.id, newDeadline);
    },
  };
};
