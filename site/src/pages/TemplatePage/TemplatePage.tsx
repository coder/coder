import { useMachine } from "@xstate/react"
import React from "react"
import { useParams } from "react-router-dom"
import { templateMachine } from "../../xServices/template/templateXService"
import { TemplatePageView } from "./TemplatePageView"

const TemplatePage: React.FC = () => {
  const { template } = useParams()
  const [templateState] = useMachine(templateMachine, {
    context: {
      name: template,
    },
  })

  return (
    <TemplatePageView
      template={templateState.context.template}
      templateVersion={templateState.context.templateVersion}
    />
  )
}

export default TemplatePage
