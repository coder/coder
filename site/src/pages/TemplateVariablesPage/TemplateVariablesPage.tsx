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

  const [state, send] = useMachine(templateVariablesMachine, {
    context: {
      organizationId,
      templateName,
    },
  })
  const {
    templateVariables,
    getTemplateError,
    getTemplateVariablesError,
    // FIXME saveTemplateVariablesError,
  } = state.context

  const { t } = useTranslation("templateVariablesPage")
  const navigate = useNavigate()
  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>

      <TemplateVariablesPageView
        isSubmitting={state.hasTag("submitting")}
        templateVariables={templateVariables}
        errors={{
          getTemplateError,
          getTemplateVariablesError,
          // FIXME saveTemplateVariablesError,
        }}
        onCancel={() => {
          navigate(`/templates/${templateName}`)
        }}
        onSubmit={(templateVariables) => {
          send({ type: "UPDATE_TEMPLATE", templateVariables })
        }}
      />

    </>
  )
}

export default TemplateVariablesPage
