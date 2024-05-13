import dayjs from "dayjs";
import { type FC, useState } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQuery, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import { API } from "api/api";
import { checkAuthorization } from "api/queries/authCheck";
import { templateByName } from "api/queries/templates";
import { workspaceByOwnerAndNameKey } from "api/queries/workspaces";
import type * as TypesGen from "api/typesGenerated";
import { Alert } from "components/Alert/Alert";
import { ErrorAlert } from "components/Alert/ErrorAlert";
import { ConfirmDialog } from "components/Dialogs/ConfirmDialog/ConfirmDialog";
import { Loader } from "components/Loader/Loader";
import { PageHeader, PageHeaderTitle } from "components/PageHeader/PageHeader";
import {
  scheduleToAutostart,
  scheduleChanged,
} from "pages/WorkspaceSettingsPage/WorkspaceSchedulePage/schedule";
import { ttlMsToAutostop } from "pages/WorkspaceSettingsPage/WorkspaceSchedulePage/ttl";
import { useWorkspaceSettings } from "pages/WorkspaceSettingsPage/WorkspaceSettingsLayout";
import { pageTitle } from "utils/page";
import {
  formValuesToAutostartRequest,
  formValuesToTTLRequest,
} from "./formToRequest";
import { WorkspaceScheduleForm } from "./WorkspaceScheduleForm";

const permissionsToCheck = (workspace: TypesGen.Workspace) =>
  ({
    updateWorkspace: {
      object: {
        resource_type: "workspace",
        resourceId: workspace.id,
        owner_id: workspace.owner_id,
      },
      action: "update",
    },
  }) as const;

export const WorkspaceSchedulePage: FC = () => {
  const params = useParams() as { username: string; workspace: string };
  const navigate = useNavigate();
  const username = params.username.replace("@", "");
  const workspaceName = params.workspace;
  const queryClient = useQueryClient();
  const workspace = useWorkspaceSettings();
  const { data: permissions, error: checkPermissionsError } = useQuery(
    checkAuthorization({ checks: permissionsToCheck(workspace) }),
  );
  const { data: template, error: getTemplateError } = useQuery(
    templateByName(workspace.organization_id, workspace.template_name),
  );
  const submitScheduleMutation = useMutation({
    mutationFn: submitSchedule,
    onSuccess: async () => {
      await queryClient.invalidateQueries(
        workspaceByOwnerAndNameKey(
          params.username.replace(/^@/, ""),
          params.workspace,
        ),
      );
    },
  });
  const error = checkPermissionsError || getTemplateError;
  const isLoading = !template || !permissions;

  const [isConfirmingApply, setIsConfirmingApply] = useState(false);
  const { mutate: updateWorkspace } = useMutation({
    mutationFn: () =>
      API.startWorkspace(workspace.id, workspace.template_active_version_id),
  });

  return (
    <>
      <Helmet>
        <title>{pageTitle([workspaceName, "Schedule"])}</title>
      </Helmet>
      <PageHeader css={{ paddingTop: 0 }}>
        <PageHeaderTitle>Workspace Schedule</PageHeaderTitle>
      </PageHeader>

      {error && <ErrorAlert error={error} />}

      {isLoading && <Loader />}

      {permissions && !permissions.updateWorkspace && (
        <Alert severity="error">
          You don&apos;t have permissions to update the schedule for this
          workspace.
        </Alert>
      )}

      {template && (
        <WorkspaceScheduleForm
          template={template}
          error={submitScheduleMutation.error}
          initialValues={{
            ...getAutostart(workspace),
            ...getAutostop(workspace),
          }}
          isLoading={submitScheduleMutation.isLoading}
          defaultTTL={dayjs.duration(template.default_ttl_ms, "ms").asHours()}
          onCancel={() => {
            navigate(`/@${username}/${workspaceName}`);
          }}
          onSubmit={async (values) => {
            const data = {
              workspace,
              autostart: formValuesToAutostartRequest(values),
              ttl: formValuesToTTLRequest(values),
              autostartChanged: scheduleChanged(
                getAutostart(workspace),
                values,
              ),
              autostopChanged: scheduleChanged(getAutostop(workspace), values),
            };

            await submitScheduleMutation.mutateAsync(data);

            if (data.autostopChanged) {
              setIsConfirmingApply(true);
            }
          }}
        />
      )}

      <ConfirmDialog
        open={isConfirmingApply}
        title="Restart workspace?"
        description="Would you like to restart your workspace now to apply your new autostop setting, or let it apply after your next workspace start?"
        confirmText="Restart"
        cancelText="Apply later"
        hideCancel={false}
        onConfirm={() => {
          updateWorkspace();
          navigate(`/@${username}/${workspaceName}`);
        }}
        onClose={() => {
          navigate(`/@${username}/${workspaceName}`);
        }}
      />
    </>
  );
};

const getAutostart = (workspace: TypesGen.Workspace) =>
  scheduleToAutostart(workspace.autostart_schedule);

const getAutostop = (workspace: TypesGen.Workspace) =>
  ttlMsToAutostop(workspace.ttl_ms);

type SubmitScheduleData = {
  workspace: TypesGen.Workspace;
  autostart: TypesGen.UpdateWorkspaceAutostartRequest;
  autostartChanged: boolean;
  ttl: TypesGen.UpdateWorkspaceTTLRequest;
  autostopChanged: boolean;
};

const submitSchedule = async (data: SubmitScheduleData) => {
  const { autostartChanged, workspace, autostart, autostopChanged, ttl } = data;
  const actions: Promise<void>[] = [];

  if (autostartChanged) {
    actions.push(API.putWorkspaceAutostart(workspace.id, autostart));
  }

  if (autostopChanged) {
    actions.push(API.putWorkspaceAutostop(workspace.id, ttl));
  }

  return Promise.all(actions);
};

export default WorkspaceSchedulePage;
