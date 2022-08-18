import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet"
import { useNavigate, useParams } from "react-router-dom"
import { useOrganizationId } from "../../hooks/useOrganizationId"
import { pageTitle } from "../../util/page"
import { createWorkspaceMachine } from "../../xServices/createWorkspace/createWorkspaceXService"
import { CreateWorkspaceErrors, CreateWorkspacePageView } from "./CreateWorkspacePageView"

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

  const {
    templates,
    templateSchema,
    selectedTemplate,
    getTemplateSchemaError,
    getTemplatesError,
    createWorkspaceError,
  } = createWorkspaceState.context

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Workspace")}</title>
      </Helmet>
      <CreateWorkspacePageView
        loadingTemplates={createWorkspaceState.matches("gettingTemplates")}
        loadingTemplateSchema={createWorkspaceState.matches("gettingTemplateSchema")}
        creatingWorkspace={createWorkspaceState.matches("creatingWorkspace")}
        hasTemplateErrors={createWorkspaceState.matches("error")}
        templateName={templateName}
        templates={templates}
        selectedTemplate={selectedTemplate}
        templateSchema={templateSchema}
        createWorkspaceErrors={{
          [CreateWorkspaceErrors.GET_TEMPLATES_ERROR]: getTemplatesError,
          [CreateWorkspaceErrors.GET_TEMPLATE_SCHEMA_ERROR]: getTemplateSchemaError,
          [CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR]: createWorkspaceError,
        }}
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
