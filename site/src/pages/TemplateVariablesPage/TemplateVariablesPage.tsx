import { useMachine } from "@xstate/react"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useParams } from "react-router-dom"
import { templateVariablesMachine } from "xServices/template/templateVariablesXService"
import { pageTitle } from "../../util/page"

export const TemplateVariablesPage: FC = () => {
  const { t } = useTranslation("templateVariablesPage")

  const { template: templateName } = useParams() as {
    organization: string
    template: string
  }
  const organizationId = useOrganizationId()

  const [state] = useMachine(templateVariablesMachine, {
    context: {
      organizationId,
      templateName,
    },
  })

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>

      <div>TODO</div>
    </>
  )
}

export default TemplateVariablesPage
