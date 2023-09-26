import { useMutation, useQueryClient } from "@tanstack/react-query";
import { updateTemplateMeta } from "api/api";
import { UpdateTemplateMeta } from "api/typesGenerated";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import { useTemplateSettings } from "../TemplateSettingsLayout";
import { TemplateSettingsPageView } from "./TemplateSettingsPageView";
import { templateByNameKey } from "api/queries/templates";
import { useOrganizationId } from "hooks";

export const TemplateSettingsPage: FC = () => {
  const { template: templateName } = useParams() as { template: string };
  const navigate = useNavigate();
  const orgId = useOrganizationId();
  const { template } = useTemplateSettings();
  const queryClient = useQueryClient();
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
      />
    </>
  );
};

export default TemplateSettingsPage;
