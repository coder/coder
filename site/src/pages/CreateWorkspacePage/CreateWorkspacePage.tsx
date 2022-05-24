import { useActor, useMachine } from "@xstate/react"
import React, { useContext } from "react"
import { useNavigate, useSearchParams } from "react-router-dom"
import { Template } from "../../api/typesGenerated"
import { createWorkspaceMachine } from "../../xServices/createWorkspace/createWorkspaceXService"
import { XServiceContext } from "../../xServices/StateContext"
import { CreateWorkspacePageView } from "./CreateWorkspacePageView"

const useOrganizationId = () => {
  const xServices = useContext(XServiceContext)
  const [authState] = useActor(xServices.authXService)
  const organizationId = authState.context.me?.organization_ids[0]

  if (!organizationId) {
    throw new Error("No organization ID found")
  }

  return organizationId
}

const CreateWorkspacePage: React.FC = () => {
  const organizationId = useOrganizationId()
  const [searchParams] = useSearchParams()
  const preSelectedTemplateName = searchParams.get("template")
  const navigate = useNavigate()
  const [createWorkspaceState, send] = useMachine(createWorkspaceMachine, {
    context: { organizationId, preSelectedTemplateName },
    actions: {
      onCreateWorkspace: (_, event) => {
        navigate("/workspaces/" + event.data.id)
      },
    },
  })

  return (
    <CreateWorkspacePageView
      loadingTemplates={createWorkspaceState.matches("gettingTemplates")}
      loadingTemplateSchema={createWorkspaceState.matches("gettingTemplateSchema")}
      creatingWorkspace={createWorkspaceState.matches("creatingWorkspace")}
      templates={createWorkspaceState.context.templates}
      selectedTemplate={createWorkspaceState.context.selectedTemplate}
      templateSchema={createWorkspaceState.context.templateSchema}
      onCancel={() => {
        navigate(preSelectedTemplateName ? "/templates" : "/workspaces")
      }}
      onSubmit={(request) => {
        send({
          type: "CREATE_WORKSPACE",
          request,
        })
      }}
      onSelectTemplate={(template: Template) => {
        send({
          type: "SELECT_TEMPLATE",
          template,
        })
      }}
    />
  )
}

export default CreateWorkspacePage
