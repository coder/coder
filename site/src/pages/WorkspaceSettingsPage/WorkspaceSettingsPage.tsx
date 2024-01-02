import { Helmet } from "react-helmet-async";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { useWorkspaceSettings } from "./WorkspaceSettingsLayout";
import { WorkspaceSettingsPageView } from "./WorkspaceSettingsPageView";
import { useMutation } from "react-query";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { patchWorkspace, updateWorkspaceAutomaticUpdates } from "api/api";
import { WorkspaceSettingsFormValues } from "./WorkspaceSettingsForm";

const WorkspaceSettingsPage = () => {
  const params = useParams() as {
    workspace: string;
    username: string;
  };
  const workspaceName = params.workspace;
  const username = params.username.replace("@", "");
  const workspace = useWorkspaceSettings();
  const navigate = useNavigate();

  const mutation = useMutation({
    mutationFn: async (formValues: WorkspaceSettingsFormValues) => {
      await Promise.all([
        patchWorkspace(workspace.id, { name: formValues.name }),
        updateWorkspaceAutomaticUpdates(
          workspace.id,
          formValues.automatic_updates,
        ),
      ]);
    },
    onSuccess: (_, formValues) => {
      displaySuccess("Workspace updated successfully");
      navigate(`/@${username}/${formValues.name}/settings`);
    },
  });

  return (
    <>
      <Helmet>
        <title>{pageTitle([workspaceName, "Settings"])}</title>
      </Helmet>

      <WorkspaceSettingsPageView
        error={mutation.error}
        workspace={workspace}
        onCancel={() => navigate(`/@${username}/${workspaceName}`)}
        onSubmit={mutation.mutateAsync}
      />
    </>
  );
};

export default WorkspaceSettingsPage;
