import { useMachine } from "@xstate/react"
import React from "react"
import { useNavigate } from "react-router"
import { useParams } from "react-router-dom"
import { createWorkspace } from "../../api/api"
import { templateMachine } from "../../xServices/template/templateXService"
import { CreateWorkspacePageView } from "./CreateWorkspacePageView"

const CreateWorkspacePage: React.FC = () => {
  const { template } = useParams()
  const [templateState] = useMachine(templateMachine, {
    context: {
      name: template,
    },
  })
  const navigate = useNavigate()
  const loading = templateState.hasTag("loading")
  if (!templateState.context.template || !templateState.context.templateSchema) {
    return null
  }

  return (
    <CreateWorkspacePageView
      template={templateState.context.template}
      templateSchema={templateState.context.templateSchema}
      loading={loading}
      onCancel={() => navigate("/templates")}
      onSubmit={async (req) => {
        if (!templateState.context.template) {
          throw new Error("template isn't valid")
        }
        const workspace = await createWorkspace(templateState.context.template.organization_id, req)
        navigate("/workspaces/" + workspace.id)
      }}
    />
  )
}

export default CreateWorkspacePage
