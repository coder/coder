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
import {
  workspaceQuota,
  workspaceResolveAutostart,
} from "api/queries/workspaceQuota";
import { useInfiniteQuery, useQuery } from "react-query";
import { infiniteWorkspaceBuilds } from "api/queries/workspaceBuilds";

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
  const { workspace, error } = workspaceState.context;
  const quotaQuery = useQuery(workspaceQuota(username));
  const pageError = error ?? quotaQuery.error;
  const buildsQuery = useInfiniteQuery({
    ...infiniteWorkspaceBuilds(workspace?.id ?? ""),
    enabled: Boolean(workspace),
  });

  const canAutostartResponse = useQuery(
    workspaceResolveAutostart(workspace?.id ?? ""),
  );

  const canAutostart = !canAutostartResponse.data?.parameter_mismatch ?? false;

  if (pageError) {
    return (
      <Margins>
        <ErrorAlert error={pageError} sx={{ my: 2 }} />
      </Margins>
    );
  }

  if (!workspace || !workspaceState.matches("ready") || !quotaQuery.isSuccess) {
    return <Loader />;
  }

  return (
    <RequirePermission
      isFeatureVisible={
        !(isAxiosError(pageError) && pageError.response?.status === 404)
      }
    >
      <WorkspaceReadyPage
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
        canAutostart={canAutostart}
      />
    </RequirePermission>
  );
};

export default WorkspacePage;
