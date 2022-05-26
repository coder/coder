import { useMachine } from "@xstate/react"
import React from "react"
import { useParams } from "react-router-dom"
import { Loader } from "../../components/Loader/Loader"
import { useOrganizationId } from "../../hooks/useOrganizationId"
import { templateMachine } from "../../xServices/template/templateXService"
import { TemplatePageView } from "./TemplatePageView"

const useTemplateName = () => {
  const { template } = useParams()

  if (!template) {
    throw new Error("No template found in the URL")
  }

  return template
}

export const TemplatePage: React.FC = () => {
  const organizationId = useOrganizationId()
  const templateName = useTemplateName()
  const [templateState] = useMachine(templateMachine, {
    context: {
      templateName,
      organizationId,
    },
  })
  const { template, activeTemplateVersion, templateResources } = templateState.context
  const isLoading = !template || !activeTemplateVersion || !templateResources

  if (isLoading) {
    return <Loader />
  }

  return (
    <TemplatePageView
      template={template}
      activeTemplateVersion={activeTemplateVersion}
      templateResources={templateResources}
    />
  )
}
