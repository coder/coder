import { useMachine } from "@xstate/react"
import React from "react"
import { useParams } from "react-router-dom"
import { templateMachine } from "../../xServices/template/templateXService"
import { CreateWorkspacePageView } from "./CreateWorkspacePageView"

const CreateWorkspacePage: React.FC = () => {
  const { template } = useParams()
  const [templateState] = useMachine(templateMachine, {
    context: {
      name: template,
    },
  })

  return (
    <CreateWorkspacePageView
      template={templateState.context.template}
      templateVersion={templateState.context.templateVersion}
    />
  )
}

export default CreateWorkspacePage
