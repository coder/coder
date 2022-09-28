import { useActor, useMachine } from "@xstate/react"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC, useContext, useState } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { createWorkspaceMachine } from "xServices/createWorkspace/createWorkspaceXService"
import { XServiceContext } from "xServices/StateContext"
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

  const xServices = useContext(XServiceContext)
  const [authState] = useActor(xServices.authXService)
  const { permissions, me } = authState.context

  const [ownerId, setOwnerId] = useState<string | undefined>(me?.id)

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
        canCreateForUser={permissions?.createWorkspaceForUser}
        defaultWorkspaceOwnerId={me?.id}
        setOwnerId={setOwnerId}
        onCancel={() => {
          navigate("/templates")
        }}
        onSubmit={(request) => {
          send({
            type: "CREATE_WORKSPACE",
            request,
            ownerId,
          })
        }}
      />
    </>
  )
}

export default CreateWorkspacePage
