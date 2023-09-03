import { useMutation } from "@tanstack/react-query"
import { updateTemplateMeta } from "api/api"
import { UpdateTemplateMeta } from "api/typesGenerated"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import { displaySuccess } from "components/GlobalSnackbar/utils"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "utils/page"
import { useTemplateSettingsContext } from "../TemplateSettingsLayout"
import { TemplateSchedulePageView } from "./TemplateSchedulePageView"
import { useLocalStorage } from "hooks"

const TemplateSchedulePage: FC = () => {
  const { template: templateName } = useParams() as { template: string }
  const navigate = useNavigate()
  const { template } = useTemplateSettingsContext()
  const { entitlements, experiments } = useDashboard()
  const allowAdvancedScheduling =
    entitlements.features["advanced_template_scheduling"].enabled
  // This check can be removed when https://github.com/coder/coder/milestone/19
  // is merged up
  const allowWorkspaceActions = experiments.includes("workspace_actions")
  const allowAutostopRequirement = experiments.includes(
    "template_autostop_requirement",
  )
  const { clearLocal } = useLocalStorage()

  const {
    mutate: updateTemplate,
    isLoading: isSubmitting,
    error: submitError,
  } = useMutation(
    (data: UpdateTemplateMeta) => updateTemplateMeta(template.id, data),
    {
      onSuccess: () => {
        displaySuccess("Template updated successfully")
        // clear browser storage of workspaces impending deletion
        clearLocal("dismissedWorkspaceList") // workspaces page
        clearLocal("dismissedWorkspace") // workspace page
      },
    },
  )

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
