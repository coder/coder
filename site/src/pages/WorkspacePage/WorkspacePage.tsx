import { Loader } from "components/Loader/Loader";
import { FC, useEffect } from "react";
import { useParams } from "react-router-dom";
import { WorkspaceReadyPage } from "./WorkspaceReadyPage";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { useOrganizationId } from "hooks";
import { Margins } from "components/Margins/Margins";
import { useInfiniteQuery, useQuery, useQueryClient } from "react-query";
import { infiniteWorkspaceBuilds } from "api/queries/workspaceBuilds";
import { templateByName } from "api/queries/templates";
import { workspaceByOwnerAndName } from "api/queries/workspaces";
import { checkAuthorization } from "api/queries/authCheck";
import { WorkspacePermissions, workspaceChecks } from "./permissions";
import { watchWorkspace } from "api/api";
import { Workspace } from "api/typesGenerated";
import { useEffectEvent } from "hooks/hookPolyfills";

export const WorkspacePage: FC = () => {
  const queryClient = useQueryClient();
  const params = useParams() as {
    username: string;
    workspace: string;
  };
  const workspaceName = params.workspace;
  const username = params.username.replace("@", "");
  const orgId = useOrganizationId();

  // Workspace
  const workspaceQueryOptions = workspaceByOwnerAndName(
    username,
    workspaceName,
  );
  const workspaceQuery = useQuery(workspaceQueryOptions);
  const workspace = workspaceQuery.data;

  // Template
  const templateQuery = useQuery({
    ...templateByName(orgId, workspace?.template_name ?? ""),
    enabled: workspace !== undefined,
  });
  const template = templateQuery.data;

  // Permissions
  const checks =
    workspace && template ? workspaceChecks(workspace, template) : {};
  const permissionsQuery = useQuery({
    ...checkAuthorization({ checks }),
    enabled: workspace !== undefined && template !== undefined,
  });
  const permissions = permissionsQuery.data as WorkspacePermissions | undefined;

  // Builds
  const buildsQuery = useInfiniteQuery({
    ...infiniteWorkspaceBuilds(workspace?.id ?? ""),
    enabled: workspace !== undefined,
  });

  // Watch workspace changes
  const updateWorkspaceData = useEffectEvent(
    async (newWorkspaceData: Workspace) => {
      queryClient.setQueryData(
        workspaceQueryOptions.queryKey,
        newWorkspaceData,
      );

      const hasNewBuild =
        newWorkspaceData.latest_build.id !== workspace!.latest_build.id;
      const lastBuildHasChanged =
        newWorkspaceData.latest_build.status !== workspace!.latest_build.status;

      if (hasNewBuild || lastBuildHasChanged) {
        await buildsQuery.refetch();
      }
    },
  );
  const workspaceId = workspace?.id;
  useEffect(() => {
    if (!workspaceId) {
      return;
    }

    const eventSource = watchWorkspace(workspaceId);

    eventSource.addEventListener("data", async (event) => {
      const newWorkspaceData = JSON.parse(event.data) as Workspace;
      await updateWorkspaceData(newWorkspaceData);
    });

    eventSource.addEventListener("error", (event) => {
      console.error("Error on getting workspace changes.", event);
    });

    return () => {
      eventSource.close();
    };
  }, [updateWorkspaceData, workspaceId]);

  // Page statuses
  const pageError =
    workspaceQuery.error ?? templateQuery.error ?? permissionsQuery.error;
  const isLoading = !workspace || !template || !permissions;

  if (pageError) {
    return (
      <Margins>
        <ErrorAlert
          error={pageError}
          css={{ marginTop: 16, marginBottom: 16 }}
        />
      </Margins>
    );
  }

  if (isLoading) {
    return <Loader />;
  }

  return (
    <WorkspaceReadyPage
      workspace={workspace}
      template={template}
      permissions={permissions}
      builds={buildsQuery.data?.pages.flat()}
      buildsError={buildsQuery.error}
      isLoadingMoreBuilds={buildsQuery.isFetchingNextPage}
      onLoadMoreBuilds={async () => {
        await buildsQuery.fetchNextPage();
      }}
      hasMoreBuilds={Boolean(buildsQuery.hasNextPage)}
    />
  );
};

export default WorkspacePage;
