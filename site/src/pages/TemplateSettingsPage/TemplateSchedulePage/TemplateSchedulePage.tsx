import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { updateTemplateMeta } from "api/api";
import type { UpdateTemplateMeta } from "api/typesGenerated";
import { templateByNameKey } from "api/queries/templates";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { useDashboard } from "components/Dashboard/DashboardProvider";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplateSchedulePageView } from "./TemplateSchedulePageView";

const TemplateSchedulePage: FC = () => {
  const { template: templateName } = useParams() as { template: string };
  const navigate = useNavigate();
  const queryClient = useQueryClient();
  const orgId = useOrganizationId();
  const { template } = useTemplateSettings();
  const { entitlements } = useDashboard();
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled;

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
        localStorage.removeItem("dismissedWorkspaceList"); // workspaces page
        localStorage.removeItem("dismissedWorkspace"); // workspace page
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
