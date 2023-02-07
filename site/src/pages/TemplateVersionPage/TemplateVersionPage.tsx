import { useMachine } from "@xstate/react"
import { useDashboard } from "components/Dashboard/DashboardProvider"
import { useOrganizationId } from "hooks/useOrganizationId"
import { useTab } from "hooks/useTab"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { templateVersionMachine } from "xServices/templateVersion/templateVersionXService"
import TemplateVersionPageView from "./TemplateVersionPageView"

type Params = {
  version: string
  template: string
}

export const TemplateVersionPage: FC = () => {
  const { version: versionName, template: templateName } = useParams() as Params
  const orgId = useOrganizationId()
  const [state] = useMachine(templateVersionMachine, {
    context: { templateName, versionName, orgId },
  })
  const tab = useTab("file", "0")
  const { t } = useTranslation("templateVersionPage")
  const dashboard = useDashboard()

  return (
    <>
      <Helmet>
        <title>
          {pageTitle(`${t("title")} ${versionName} · ${templateName}`)}
        </title>
      </Helmet>

      <TemplateVersionPageView
        context={state.context}
        versionName={versionName}
        templateName={templateName}
        tab={tab}
        canEdit={dashboard.experiments.includes("template_editor")}
      />
    </>
  )
}

export default TemplateVersionPage
