import { useMutation, useQueryClient } from "@tanstack/react-query";
import { updateTemplateMeta } from "api/api";
import { UpdateTemplateMeta } from "api/typesGenerated";
import { displaySuccess } from "components/GlobalSnackbar/utils";
import { FC } from "react";
import { Helmet } from "react-helmet-async";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router-dom";
import { pageTitle } from "utils/page";
import {
  getTemplateQuery,
  useTemplateSettingsContext,
} from "../TemplateSettingsLayout";
import { TemplateSettingsPageView } from "./TemplateSettingsPageView";

export const TemplateSettingsPage: FC = () => {
  const { template: templateName } = useParams() as { template: string };
  const { t } = useTranslation("templateSettingsPage");
  const navigate = useNavigate();
  const { template } = useTemplateSettingsContext();
  const queryClient = useQueryClient();
  const {
    mutate: updateTemplate,
    isLoading: isSubmitting,
    error: submitError,
  } = useMutation(
    (data: UpdateTemplateMeta) => updateTemplateMeta(template.id, data),
    {
      onSuccess: async () => {
        await queryClient.invalidateQueries({
          queryKey: getTemplateQuery(templateName),
        });
        displaySuccess("Template updated successfully");
      },
    },
  );

  return (
    <>
      <Helmet>
        <title>{pageTitle([template.name, t("title")])}</title>
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
