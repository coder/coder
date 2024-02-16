import { type FC } from "react";
import { Helmet } from "react-helmet-async";
import { useMutation, useQueryClient } from "react-query";
import { useNavigate, useParams } from "react-router-dom";
import { updateTemplateMeta } from "api/api";
import type { UpdateTemplateMeta } from "api/typesGenerated";
import { templateByNameKey } from "api/queries/templates";
import { useOrganizationId } from "contexts/auth/useOrganizationId";
import { useDashboard } from "modules/dashboard/useDashboard";
import { pageTitle } from "utils/page";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplateSettingsPageView } from "./TemplateSettingsPageView";

export const TemplateSettingsPage: FC = () => {
  const { template: templateName } = useParams() as { template: string };
  const navigate = useNavigate();
  const orgId = useOrganizationId();
  const { template } = useTemplateSettings();
  const queryClient = useQueryClient();
  const { entitlements, experiments } = useDashboard();
  const accessControlEnabled = entitlements.features.access_control.enabled;
  const sharedPortsExperimentEnabled = experiments.includes("shared-ports");
  const sharedPortControlsEnabled =
    entitlements.features.control_shared_ports.enabled;

  const {
    mutate: updateTemplate,
    isLoading: isSubmitting,
    error: submitError,
  } = useMutation(
    (data: UpdateTemplateMeta) => updateTemplateMeta(template.id, data),
    {
      onSuccess: async (data) => {
        // This update has a chance to return a 304 which means nothing was updated.
        // In this case, the return payload will be empty and we should use the
        // original template data.
        if (!data) {
          data = template;
        } else {
          // Only invalid the query if data is returned, indicating at least one field was updated.
          //
          // we use data.name because an admin may have updated templateName to something new
          await queryClient.invalidateQueries(
            templateByNameKey(orgId, data.name),
          );
        }
        displaySuccess("Template updated successfully");
        navigate(`/templates/${data.name}`);
      },
    },
  );

  return (
    <>
      <Helmet>
        <title>{pageTitle([template.name, "General Settings"])}</title>
      </Helmet>
      <TemplateSettingsPageView
        isSubmitting={isSubmitting}
        template={template}
        submitError={submitError}
        onCancel={() => {
          navigate(`/templates/${templateName}`);
        }}
        onSubmit={(templateSettings) => {
          updateTemplate({
            ...template,
            ...templateSettings,
          });
        }}
        accessControlEnabled={accessControlEnabled}
        sharedPortsExperimentEnabled={sharedPortsExperimentEnabled}
        sharedPortControlsEnabled={sharedPortControlsEnabled}
      />
    </>
  );
};

export default TemplateSettingsPage;
