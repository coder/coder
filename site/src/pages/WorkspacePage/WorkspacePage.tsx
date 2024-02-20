import { type FC, useEffect } from "react";
import { useQuery, useQueryClient } from "react-query";
import { useParams } from "react-router-dom";
import { watchWorkspace } from "api/api";
import type { Workspace } from "api/typesGenerated";
import { workspaceBuildsKey } from "api/queries/workspaceBuilds";
import { templateByName } from "api/queries/templates";
import { workspaceByOwnerAndName } from "api/queries/workspaces";
import { checkAuthorization } from "api/queries/authCheck";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { Navbar } from "modules/dashboard/Navbar/Navbar";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { Loader } from "components/Loader/Loader";
import { Margins } from "components/Margins/Margins";
import { WorkspacePermissions, workspaceChecks } from "./permissions";
import { WorkspaceReadyPage } from "./WorkspaceReadyPage";

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

  // Watch workspace changes
  const updateWorkspaceData = useEffectEvent(
    async (newWorkspaceData: Workspace) => {
      if (!workspace) {
        throw new Error(
          "Applying an update for a workspace that is undefined.",
        );
      }

      queryClient.setQueryData(
        workspaceQueryOptions.queryKey,
        newWorkspaceData,
      );

      const hasNewBuild =
        newWorkspaceData.latest_build.id !== workspace.latest_build.id;
      const lastBuildHasChanged =
        newWorkspaceData.latest_build.status !== workspace.latest_build.status;

      if (hasNewBuild || lastBuildHasChanged) {
        await queryClient.invalidateQueries(
          workspaceBuildsKey(newWorkspaceData.id),
        );
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

  return (
    <div css={{ height: "100%", display: "flex", flexDirection: "column" }}>
      <Navbar />
      {pageError ? (
        <Margins>
          <ErrorAlert
            error={pageError}
            css={{ marginTop: 16, marginBottom: 16 }}
          />
        </Margins>
      ) : isLoading ? (
        <Loader />
      ) : (
        <WorkspaceReadyPage
          workspace={workspace}
          template={template}
          permissions={permissions}
        />
      )}
    </div>
  );
};

export default WorkspacePage;
