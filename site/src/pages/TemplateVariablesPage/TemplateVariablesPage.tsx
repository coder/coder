import { useMachine } from "@xstate/react"
import {
  CreateTemplateVersionRequest,
  TemplateVersionVariable,
  VariableValue,
} from "api/typesGenerated"
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
    getTemplateDataError,
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
          getTemplateDataError,
          updateTemplateError,
        }}
        onCancel={() => {
          navigate(`/templates/${templateName}`)
        }}
        onSubmit={(formData) => {
          const request = filterEmptySensitiveVariables(
            formData,
            templateVariables,
          )
          send({ type: "UPDATE_TEMPLATE_EVENT", request: request })
        }}
      />
    </>
  )
}

const filterEmptySensitiveVariables = (
  request: CreateTemplateVersionRequest,
  templateVariables?: TemplateVersionVariable[],
): CreateTemplateVersionRequest => {
  const filtered: VariableValue[] = []

  if (!templateVariables) {
    return request
  }

  if (request.user_variable_values) {
    request.user_variable_values.forEach((variableValue) => {
      const templateVariable = templateVariables.find(
        (t) => t.name === variableValue.name,
      )
      if (
        templateVariable &&
        templateVariable.sensitive &&
        variableValue.value === ""
      ) {
        return
      }
      filtered.push(variableValue)
    })
  }

  return {
    ...request,
    user_variable_values: filtered,
  }
}

export default TemplateVariablesPage
