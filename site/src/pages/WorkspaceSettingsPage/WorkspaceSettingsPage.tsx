import { Helmet } from "react-helmet-async";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { useWorkspaceSettings } from "./WorkspaceSettingsLayout";
import { WorkspaceSettingsPageView } from "./WorkspaceSettingsPageView";
import { useMutation } from "@tanstack/react-query";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { patchWorkspace } from "api/api";
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
    mutationFn: (formValues: WorkspaceSettingsFormValues) =>
      patchWorkspace(workspace.id, { name: formValues.name }),
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
        isSubmitting={mutation.isLoading}
        workspace={workspace}
        onCancel={() => navigate(`/@${username}/${workspaceName}`)}
        onSubmit={mutation.mutate}
      />
    </>
  );
};

export default WorkspaceSettingsPage;
