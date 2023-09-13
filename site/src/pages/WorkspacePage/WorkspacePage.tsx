import { useMachine } from "@xstate/react";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
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
import { useQuery } from "@tanstack/react-query";

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
  });
  const { workspace, error } = workspaceState.context;
  const quotaQuery = useQuery(workspaceQuota(username));
  const pageError = error ?? quotaQuery.error;

  return (
    <RequirePermission
      isFeatureVisible={
        !(isAxiosError(pageError) && pageError.response?.status === 404)
      }
    >
      <ChooseOne>
        <Cond condition={Boolean(pageError)}>
          <Margins>
            <ErrorAlert error={pageError} sx={{ my: 2 }} />
          </Margins>
        </Cond>
        <Cond
          condition={
            Boolean(workspace) &&
            workspaceState.matches("ready") &&
            quotaQuery.isSuccess
          }
        >
          <WorkspaceReadyPage
            workspaceState={workspaceState}
            quota={quotaQuery.data}
            workspaceSend={workspaceSend}
          />
        </Cond>
        <Cond>
          <Loader />
        </Cond>
      </ChooseOne>
    </RequirePermission>
  );
};

export default WorkspacePage;
