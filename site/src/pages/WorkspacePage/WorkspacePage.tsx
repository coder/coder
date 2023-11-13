import { useMachine } from "@xstate/react";
import { Loader } from "components/Loader/Loader";
import { FC, useEffect, useRef } from "react";
import { useParams } from "react-router-dom";
import { workspaceMachine } from "xServices/workspace/workspaceXService";
import { WorkspaceReadyPage } from "./WorkspaceReadyPage";
import { RequirePermission } from "components/RequirePermission/RequirePermission";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { useOrganizationId } from "hooks";
import { isAxiosError } from "axios";
import { Margins } from "components/Margins/Margins";
import { useInfiniteQuery, useQuery, useQueryClient } from "react-query";
import { infiniteWorkspaceBuilds } from "api/queries/workspaceBuilds";
import { templateByName } from "api/queries/templates";
import { workspaceByOwnerAndName } from "api/queries/workspaces";
import { checkAuthorization } from "api/queries/authCheck";
import { WorkspacePermissions, workspaceChecks } from "./permissions";
import { watchWorkspace } from "api/api";
import { Workspace } from "api/typesGenerated";

export const WorkspacePage: FC = () => {
  const queryClient = useQueryClient();
  const params = useParams() as {
    username: string;
    workspace: string;
  };
  const workspaceName = params.workspace;
  const username = params.username.replace("@", "");
  const orgId = useOrganizationId();
  const [workspaceState, workspaceSend] = useMachine(workspaceMachine, {
    context: {
      orgId,
      workspaceName,
      username,
    },
    actions: {
      refreshBuilds: async () => {
        await buildsQuery.refetch();
      },
    },
  });

  const workspaceQueryOptions = workspaceByOwnerAndName(
    username,
    workspaceName,
  );
  const workspaceQuery = useQuery(workspaceQueryOptions);
  const workspace = workspaceQuery.data;

  const templateQuery = useQuery({
    ...templateByName(orgId, workspace?.template_name ?? ""),
    enabled: workspace !== undefined,
  });
  const template = templateQuery.data;

  const checks =
    workspace && template ? workspaceChecks(workspace, template) : {};
  const permissionsQuery = useQuery({
    ...checkAuthorization({ checks }),
    enabled: workspace !== undefined && template !== undefined,
  });
  const permissions = permissionsQuery.data as WorkspacePermissions | undefined;

  const buildsQuery = useInfiniteQuery({
    ...infiniteWorkspaceBuilds(workspace?.id ?? ""),
    enabled: workspace !== undefined,
  });

  const pageError =
    workspaceQuery.error ?? templateQuery.error ?? permissionsQuery.error;
  const isLoading = !workspace || !template || !permissions;

  // Watch workspace changes
  const workspaceEventSource = useRef<EventSource | null>(null);
  useEffect(() => {
    // If there is an event source, we are already watching the workspace
    if (!workspace || workspaceEventSource.current) {
      return;
    }

    const eventSource = watchWorkspace(workspace.id);
    workspaceEventSource.current = eventSource;

    eventSource.addEventListener("data", async (event) => {
      const newWorkspaceData = JSON.parse(event.data) as Workspace;
      queryClient.setQueryData(
        workspaceQueryOptions.queryKey,
        newWorkspaceData,
      );

      const hasNewBuild =
        newWorkspaceData.latest_build.id !== workspace.latest_build.id;
      const lastBuildHasChanged =
        newWorkspaceData.latest_build.status !== workspace.latest_build.status;

      if (hasNewBuild || lastBuildHasChanged) {
        await buildsQuery.refetch();
      }
    });

    eventSource.addEventListener("error", (event) => {
      console.error("Error on getting workspace changes.", event);
    });

    return () => {
      eventSource.close();
    };
  }, [buildsQuery, queryClient, workspace, workspaceQueryOptions.queryKey]);

  if (pageError) {
    return (
      <Margins>
        <ErrorAlert error={pageError} sx={{ my: 2 }} />
      </Margins>
    );
  }

  if (isLoading) {
    return <Loader />;
  }

  return (
    <RequirePermission
      isFeatureVisible={
        !(isAxiosError(pageError) && pageError.response?.status === 404)
      }
    >
      <WorkspaceReadyPage
        workspace={workspace}
        template={template}
        permissions={permissions}
        workspaceState={workspaceState}
        workspaceSend={workspaceSend}
        builds={buildsQuery.data?.pages.flat()}
        buildsError={buildsQuery.error}
        isLoadingMoreBuilds={buildsQuery.isFetchingNextPage}
        onLoadMoreBuilds={async () => {
          await buildsQuery.fetchNextPage();
        }}
        hasMoreBuilds={Boolean(buildsQuery.hasNextPage)}
      />
    </RequirePermission>
  );
};

export default WorkspacePage;
