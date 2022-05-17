import { useMachine } from "@xstate/react"
import React from "react"
import { templatesMachine } from "../../xServices/templates/templatesXService"
import { TemplatesPageView } from "./TemplatesPageView"

const TemplatesPage: React.FC = () => {
  const [templatesState] = useMachine(templatesMachine)

  return (
    <TemplatesPageView
      templates={templatesState.context.templates}
      canCreateTemplate={templatesState.context.canCreateTemplate}
      loading={templatesState.hasTag("loading")}
    />
  )
}

export default TemplatesPage
