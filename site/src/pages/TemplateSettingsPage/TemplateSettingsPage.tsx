import { useMachine } from "@xstate/react"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC } from "react"
import { Helmet } from "react-helmet"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { templateSettingsMachine } from "xServices/templateSettings/templateSettingsXService"
import { TemplateSettingsPageView } from "./TemplateSettingsPageView"

const Language = {
  title: "Template Settings",
}

export const TemplateSettingsPage: FC = () => {
  const { template: templateName } = useParams() as { template: string }
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
  const { templateSettings: template, saveTemplateSettingsError, getTemplateError } = state.context

  return (
    <>
      <Helmet>
        <title>{pageTitle(Language.title)}</title>
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
