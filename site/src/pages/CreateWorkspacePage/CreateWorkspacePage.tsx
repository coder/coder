import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet"
import { useNavigate, useSearchParams } from "react-router-dom"
import { Template } from "../../api/typesGenerated"
import { useOrganizationId } from "../../hooks/useOrganizationId"
import { pageTitle } from "../../util/page"
import { createWorkspaceMachine } from "../../xServices/createWorkspace/createWorkspaceXService"
import { CreateWorkspacePageView } from "./CreateWorkspacePageView"

const CreateWorkspacePage: FC = () => {
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
    <>
      <Helmet>
        <title>{pageTitle("Create Workspace")}</title>
      </Helmet>
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
    </>
  )
}

export default CreateWorkspacePage
