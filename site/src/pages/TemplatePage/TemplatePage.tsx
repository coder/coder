import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet"
import { useParams } from "react-router-dom"
import { Loader } from "../../components/Loader/Loader"
import { useOrganizationId } from "../../hooks/useOrganizationId"
import { pageTitle } from "../../util/page"
import { templateMachine } from "../../xServices/template/templateXService"
import { TemplatePageView } from "./TemplatePageView"

const useTemplateName = () => {
  const { template } = useParams()

  if (!template) {
    throw new Error("No template found in the URL")
  }

  return template
}

export const TemplatePage: FC<React.PropsWithChildren<unknown>> = () => {
  const organizationId = useOrganizationId()
  const templateName = useTemplateName()
  const [templateState] = useMachine(templateMachine, {
    context: {
      templateName,
      organizationId,
    },
  })
  const { template, activeTemplateVersion, templateResources, templateVersions } =
    templateState.context
  const isLoading = !template || !activeTemplateVersion || !templateResources

  if (isLoading) {
    return <Loader />
  }

  return (
    <>
      <Helmet>
        <title>{pageTitle(`${template.name} · Template`)}</title>
      </Helmet>
      <TemplatePageView
        template={template}
        activeTemplateVersion={activeTemplateVersion}
        templateResources={templateResources}
        templateVersions={templateVersions}
      />
    </>
  )
}
