import { useMutation } from "@tanstack/react-query"
import { updateTemplateMeta } from "api/api"
import { UpdateTemplateMeta } from "api/typesGenerated"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import { displaySuccess } from "components/GlobalSnackbar/utils"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { useTemplateSettingsContext } from "../TemplateSettingsLayout"
import { TemplateSchedulePageView } from "./TemplateSchedulePageView"

const TemplateSchedulePage: FC = () => {
  const { template: templateName } = useParams() as { template: string }
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
      onSuccess: () => {
        displaySuccess("Template updated successfully")
      },
    },
  )

  return (
    <>
      <Helmet>
        <title>{pageTitle([template.name, "Schedule"])}</title>
      </Helmet>
      <TemplateSchedulePageView
        canSetMaxTTL={canSetMaxTTL}
        isSubmitting={isSubmitting}
        template={template}
        submitError={submitError}
        onCancel={() => {
          navigate(`/templates/${templateName}`)
        }}
        onSubmit={(templateScheduleSettings) => {
          updateTemplate({
            ...template,
            ...templateScheduleSettings,
          })
        }}
      />
    </>
  )
}

export default TemplateSchedulePage
