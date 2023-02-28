import { useMachine } from "@xstate/react"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useNavigate, useParams } from "react-router-dom"
import { templateVariablesMachine } from "xServices/template/templateVariablesXService"
import { pageTitle } from "../../util/page"
import { TemplateVariablesPageView } from "./TemplateVariablesPageView"

export const TemplateVariablesPage: FC = () => {
  const { template: templateName } = useParams() as {
    organization: string
    template: string
  }
  const organizationId = useOrganizationId()
  const navigate = useNavigate()
  const [state, send] = useMachine(templateVariablesMachine, {
    context: {
      organizationId,
      templateName,
    },
    actions: {
      onUpdateTemplate: () => {
        navigate(`/templates/${templateName}`)
      },
    },
  })
  const {
    activeTemplateVersion,
    templateVariables,
    getTemplateError,
    getActiveTemplateVersionError,
    getTemplateVariablesError,
    updateTemplateError,
  } = state.context

  const { t } = useTranslation("templateVariablesPage")
  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>

      <TemplateVariablesPageView
        isSubmitting={state.hasTag("submitting")}
        templateVersion={activeTemplateVersion}
        templateVariables={templateVariables}
        errors={{
          getTemplateError,
          getActiveTemplateVersionError,
          getTemplateVariablesError,
          updateTemplateError,
        }}
        onCancel={() => {
          navigate(`/templates/${templateName}`)
        }}
        onSubmit={(formData) => {
          send({ type: "UPDATE_TEMPLATE_EVENT", request: formData })
        }}
      />
    </>
  )
}

export default TemplateVariablesPage
