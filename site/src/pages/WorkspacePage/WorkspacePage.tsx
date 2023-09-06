import { useMachine } from "@xstate/react";
import { ChooseOne, Cond } from "components/Conditionals/ChooseOne";
import { Loader } from "components/Loader/Loader";
import { FC } from "react";
import { useParams } from "react-router-dom";
import { quotaMachine } from "xServices/quotas/quotasXService";
import { workspaceMachine } from "xServices/workspace/workspaceXService";
import { WorkspaceReadyPage } from "./WorkspaceReadyPage";
import { RequirePermission } from "components/RequirePermission/RequirePermission";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { useOrganizationId } from "hooks";
import { isAxiosError } from "axios";
import { Margins } from "components/Margins/Margins";

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
  const [quotaState] = useMachine(quotaMachine, { context: { username } });
  const { getQuotaError } = quotaState.context;
  const pageError = error ?? getQuotaError;

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
            quotaState.matches("success")
          }
        >
          <WorkspaceReadyPage
            workspaceState={workspaceState}
            quotaState={quotaState}
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
