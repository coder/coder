import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet"
import { useNavigate, useParams } from "react-router-dom"
import { useOrganizationId } from "../../hooks/useOrganizationId"
import { pageTitle } from "../../util/page"
import { createWorkspaceMachine } from "../../xServices/createWorkspace/createWorkspaceXService"
import { CreateWorkspacePageView } from "./CreateWorkspacePageView"

const CreateWorkspacePage: FC = () => {
  const organizationId = useOrganizationId()
  const { template } = useParams()
  const templateName = template ? template : ""
  const navigate = useNavigate()
  const [createWorkspaceState, send] = useMachine(createWorkspaceMachine, {
    context: { organizationId, templateName },
    actions: {
      onCreateWorkspace: (_, event) => {
        navigate(`/@${event.data.owner_name}/${event.data.name}`)
      },
    },
  })

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Workspace")}</title>
      </Helmet>
      <CreateWorkspacePageView
        loadingTemplates={createWorkspaceState.matches("gettingTemplates")}
        loadingTemplateSchema={createWorkspaceState.matches("gettingTemplateSchema")}
        creatingWorkspace={createWorkspaceState.matches("creatingWorkspace")}
        templateName={createWorkspaceState.context.templateName}
        templates={createWorkspaceState.context.templates}
        selectedTemplate={createWorkspaceState.context.selectedTemplate}
        templateSchema={createWorkspaceState.context.templateSchema}
        onCancel={() => {
          navigate("/templates")
        }}
        onSubmit={(request) => {
          send({
            type: "CREATE_WORKSPACE",
            request,
          })
        }}
      />
    </>
  )
}

export default CreateWorkspacePage
