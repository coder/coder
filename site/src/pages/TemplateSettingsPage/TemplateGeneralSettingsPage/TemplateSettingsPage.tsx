import { useMutation } from "@tanstack/react-query"
import { updateTemplateMeta } from "api/api"
import { UpdateTemplateMeta } from "api/typesGenerated"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import { displaySuccess } from "components/GlobalSnackbar/utils"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { useTemplateSettingsContext } from "../TemplateSettingsLayout"
import { TemplateSettingsPageView } from "./TemplateSettingsPageView"

export const TemplateSettingsPage: FC = () => {
  const { template: templateName } = useParams() as { template: string }
  const { t } = useTranslation("templateSettingsPage")
  const navigate = useNavigate()
  const { template } = useTemplateSettingsContext()
  const { entitlements } = useDashboard()
  const canSetMaxTTL =
    entitlements.features["advanced_template_scheduling"].enabled
  const {
    mutate: updateTemplate,
    isLoading: isSubmitting,
    error: submitError,
  } = useMutation(
    (data: UpdateTemplateMeta) => updateTemplateMeta(template.id, data),
    {
      onSuccess: async () => {
        displaySuccess("Template updated successfully")
      },
    },
  )

  return (
    <>
      <Helmet>
        <title>{pageTitle([template.name, t("title")])}</title>
      </Helmet>
      <TemplateSettingsPageView
        canSetMaxTTL={canSetMaxTTL}
        isSubmitting={isSubmitting}
        template={template}
        submitError={submitError}
        onCancel={() => {
          navigate(`/templates/${templateName}`)
        }}
        onSubmit={(templateSettings) => {
          updateTemplate({
            ...template,
            ...templateSettings,
          })
        }}
      />
    </>
  )
}
