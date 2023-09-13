import { useMutation, useQueryClient } from "@tanstack/react-query";
import { updateTemplateMeta } from "api/api";
import { UpdateTemplateMeta } from "api/typesGenerated";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplateSchedulePageView } from "./TemplateSchedulePageView";
import { useLocalStorage, useOrganizationId } from "hooks";
import { templateByNameKey } from "api/queries/templates";

const TemplateSchedulePage: FC = () => {
  const { template: templateName } = useParams() as { template: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const orgId = useOrganizationId();
  const { template } = useTemplateSettings();
  const { entitlements, experiments } = useDashboard();
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled;
  // This check can be removed when https://github.com/coder/coder/milestone/19
  // is merged up
  const allowWorkspaceActions = experiments.includes("workspace_actions");
  const allowAutostopRequirement =
    entitlements.features["template_autostop_requirement"].enabled;
  const { clearLocal } = useLocalStorage();

  const {
    mutate: updateTemplate,
    isLoading: isSubmitting,
    error: submitError,
  } = useMutation(
    (data: UpdateTemplateMeta) => updateTemplateMeta(template.id, data),
    {
      onSuccess: async () => {
        await queryClient.invalidateQueries(
          templateByNameKey(orgId, templateName),
        );
        displaySuccess("Template updated successfully");
        // clear browser storage of workspaces impending deletion
        clearLocal("dismissedWorkspaceList"); // workspaces page
        clearLocal("dismissedWorkspace"); // workspace page
      },
    },
  );

  return (
    <>
      <Helmet>
        <title>{pageTitle([template.name, "Schedule"])}</title>
      </Helmet>
      <TemplateSchedulePageView
        allowAdvancedScheduling={allowAdvancedScheduling}
        allowWorkspaceActions={allowWorkspaceActions}
        allowAutostopRequirement={allowAutostopRequirement}
        isSubmitting={isSubmitting}
        template={template}
        submitError={submitError}
        onCancel={() => {
          navigate(`/templates/${templateName}`);
        }}
        onSubmit={(templateScheduleSettings) => {
          updateTemplate({
            ...template,
            ...templateScheduleSettings,
          });
        }}
      />
    </>
  );
};

export default TemplateSchedulePage;
