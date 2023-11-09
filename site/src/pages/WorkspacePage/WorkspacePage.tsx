import { useMachine } from "@xstate/react";
import { Loader } from "components/Loader/Loader";
import { FC } from "react";
import { useParams } from "react-router-dom";
import { workspaceMachine } from "xServices/workspace/workspaceXService";
import { WorkspaceReadyPage } from "./WorkspaceReadyPage";
import { RequirePermission } from "components/RequirePermission/RequirePermission";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { useOrganizationId } from "hooks";
import { isAxiosError } from "axios";
import { Margins } from "components/Margins/Margins";
import { workspaceQuota } from "api/queries/workspaceQuota";
import { useInfiniteQuery, useQuery } from "react-query";
import { infiniteWorkspaceBuilds } from "api/queries/workspaceBuilds";
import { templateByName } from "api/queries/templates";
import { workspaceByOwnerAndName } from "api/queries/workspaces";
import { checkAuthorization } from "api/queries/authCheck";
import { WorkspacePermissions, workspaceChecks } from "./permissions";

export const WorkspacePage: FC = () => {
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

  const workspaceQuery = useQuery(
    workspaceByOwnerAndName(username, workspaceName),
  );
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

  const quotaQuery = useQuery(workspaceQuota(username));

  const buildsQuery = useInfiniteQuery({
    ...infiniteWorkspaceBuilds(workspace?.id ?? ""),
    enabled: workspace !== undefined,
  });

  const pageError =
    workspaceQuery.error ??
    templateQuery.error ??
    quotaQuery.error ??
    permissionsQuery.error;
  const isLoading = !workspace || !template || !permissions || !quotaQuery.data;

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
        quota={quotaQuery.data}
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
