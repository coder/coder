import { useMachine } from "@xstate/react"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { templateSettingsMachine } from "xServices/templateSettings/templateSettingsXService"
import { TemplateSettingsPageView } from "./TemplateSettingsPageView"

export const TemplateSettingsPage: FC = () => {
  const { template: templateName } = useParams() as { template: string }
  const { t } = useTranslation("templateSettingsPage")
  const navigate = useNavigate()
  const organizationId = useOrganizationId()
  const [state, send] = useMachine(templateSettingsMachine, {
    context: { templateName, organizationId },
    actions: {
      onSave: (_, { data }) => {
        // Use the data.name because the template name can be changed
        navigate(`/templates/${data.name}`)
      },
    },
  })
  const {
    templateSettings: template,
    saveTemplateSettingsError,
    getTemplateError,
  } = state.context

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>
      <TemplateSettingsPageView
        isSubmitting={state.hasTag("submitting")}
        template={template}
        errors={{
          getTemplateError,
          saveTemplateSettingsError,
        }}
        onCancel={() => {
          navigate(`/templates/${templateName}`)
        }}
        onSubmit={(templateSettings) => {
          send({ type: "SAVE", templateSettings })
        }}
      />
    </>
  )
}
