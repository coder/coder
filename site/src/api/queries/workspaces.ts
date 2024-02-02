import * as API from "api/api";
import {
  QueryClient,
  UseMutationOptions,
  type QueryOptions,
} from "react-query";
import { putWorkspaceExtension } from "api/api";
import { Dayjs } from "dayjs";
import {
  type WorkspaceBuildParameter,
  type Workspace,
  type CreateWorkspaceRequest,
  type WorkspacesResponse,
  type WorkspacesRequest,
  WorkspaceBuild,
  ProvisionerLogLevel,
} from "api/typesGenerated";
import { workspaceBuildsKey } from "./workspaceBuilds";

export const workspaceByOwnerAndNameKey = (owner: string, name: string) => [
  "workspace",
  owner,
  name,
  "settings",
];

export const workspaceByOwnerAndName = (owner: string, name: string) => {
  return {
    queryKey: workspaceByOwnerAndNameKey(owner, name),
    queryFn: () =>
      API.getWorkspaceByOwnerAndName(owner, name, { include_deleted: true }),
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

export const updateDeadline = (
  workspace: Workspace,
): UseMutationOptions<void, unknown, Dayjs> => {
  return {
    mutationFn: (deadline: Dayjs) => {
      return putWorkspaceExtension(workspace.id, deadline);
    },
  };
};

export const changeVersion = (
  workspace: Workspace,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: ({
      versionId,
      buildParameters,
    }: {
      versionId: string;
      buildParameters?: WorkspaceBuildParameter[];
    }) => {
      return API.changeWorkspaceVersion(workspace, versionId, buildParameters);
    },
    onSuccess: async (build: WorkspaceBuild) => {
      await updateWorkspaceBuild(build, queryClient);
    },
  };
};

export const updateWorkspace = (
  workspace: Workspace,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: (buildParameters?: WorkspaceBuildParameter[]) => {
      return API.updateWorkspace(workspace, buildParameters);
    },
    onSuccess: async (build: WorkspaceBuild) => {
      await updateWorkspaceBuild(build, queryClient);
    },
  };
};

export const deleteWorkspace = (
  workspace: Workspace,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: (options: API.DeleteWorkspaceOptions) => {
      return API.deleteWorkspace(workspace.id, options);
    },
    onSuccess: async (build: WorkspaceBuild) => {
      await updateWorkspaceBuild(build, queryClient);
    },
  };
};

export const stopWorkspace = (
  workspace: Workspace,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: ({ logLevel }: { logLevel?: ProvisionerLogLevel }) => {
      return API.stopWorkspace(workspace.id, logLevel);
    },
    onSuccess: async (build: WorkspaceBuild) => {
      await updateWorkspaceBuild(build, queryClient);
    },
  };
};

export const startWorkspace = (
  workspace: Workspace,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: ({
      buildParameters,
      logLevel,
    }: {
      buildParameters?: WorkspaceBuildParameter[];
      logLevel?: ProvisionerLogLevel;
    }) => {
      return API.startWorkspace(
        workspace.id,
        workspace.latest_build.template_version_id,
        logLevel,
        buildParameters,
      );
    },
    onSuccess: async (build: WorkspaceBuild) => {
      await updateWorkspaceBuild(build, queryClient);
    },
  };
};

export const cancelBuild = (workspace: Workspace, queryClient: QueryClient) => {
  return {
    mutationFn: () => {
      return API.cancelWorkspaceBuild(workspace.latest_build.id);
    },
    onSuccess: async () => {
      await queryClient.invalidateQueries({
        queryKey: workspaceBuildsKey(workspace.id),
      });
    },
  };
};

export const activate = (workspace: Workspace, queryClient: QueryClient) => {
  return {
    mutationFn: () => {
      return API.updateWorkspaceDormancy(workspace.id, false);
    },
    onSuccess: (updatedWorkspace: Workspace) => {
      queryClient.setQueryData(
        workspaceByOwnerAndNameKey(workspace.owner_name, workspace.name),
        updatedWorkspace,
      );
    },
  };
};

const updateWorkspaceBuild = async (
  build: WorkspaceBuild,
  queryClient: QueryClient,
) => {
  const workspaceKey = workspaceByOwnerAndNameKey(
    build.workspace_owner_name,
    build.workspace_name,
  );
  const previousData = queryClient.getQueryData(workspaceKey) as Workspace;

  // Check if the build returned is newer than the previous build that could be
  // updated from web socket
  const previousUpdate = new Date(previousData.latest_build.updated_at);
  const newestUpdate = new Date(build.updated_at);
  if (newestUpdate > previousUpdate) {
    queryClient.setQueryData(workspaceKey, {
      ...previousData,
      latest_build: build,
    });
  }

  await queryClient.invalidateQueries({
    queryKey: workspaceBuildsKey(build.workspace_id),
  });
};

export const toggleFavorite = (
  workspace: Workspace,
  queryClient: QueryClient,
) => {
  return {
    mutationFn: () => {
      if (workspace.favorite) {
        return API.deleteFavoriteWorkspace(workspace.id);
      } else {
        return API.putFavoriteWorkspace(workspace.id);
      }
    },
    onSuccess: async () => {
      queryClient.setQueryData(
        workspaceByOwnerAndNameKey(workspace.owner_name, workspace.name),
        { ...workspace, favorite: !workspace.favorite },
      );
      await queryClient.invalidateQueries({
        queryKey: workspaceByOwnerAndNameKey(
          workspace.owner_name,
          workspace.name,
        ),
      });
    },
  };
};
