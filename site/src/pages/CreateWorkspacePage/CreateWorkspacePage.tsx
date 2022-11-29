import { useActor, useMachine } from "@xstate/react"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC, useContext } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { createWorkspaceMachine } from "xServices/createWorkspace/createWorkspaceXService"
import { XServiceContext } from "xServices/StateContext"
import {
  CreateWorkspaceErrors,
  CreateWorkspacePageView,
} from "./CreateWorkspacePageView"

const CreateWorkspacePage: FC = () => {
  const xServices = useContext(XServiceContext)
  const organizationId = useOrganizationId()
  const { template } = useParams()
  const templateName = template ? template : ""
  const navigate = useNavigate()
  const [authState] = useActor(xServices.authXService)
  const { me } = authState.context
  const [createWorkspaceState, send] = useMachine(createWorkspaceMachine, {
    context: {
      organizationId,
      templateName,
      owner: me ?? null,
    },
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
    permissions,
    owner,
  } = createWorkspaceState.context

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Workspace")}</title>
      </Helmet>
      <CreateWorkspacePageView
        loadingTemplates={createWorkspaceState.matches("gettingTemplates")}
        loadingTemplateSchema={createWorkspaceState.matches(
          "gettingTemplateSchema",
        )}
        creatingWorkspace={createWorkspaceState.matches("creatingWorkspace")}
        hasTemplateErrors={createWorkspaceState.matches("error")}
        templateName={templateName}
        templates={templates}
        selectedTemplate={selectedTemplate}
        templateSchema={templateSchema}
        createWorkspaceErrors={{
          [CreateWorkspaceErrors.GET_TEMPLATES_ERROR]: getTemplatesError,
          [CreateWorkspaceErrors.GET_TEMPLATE_SCHEMA_ERROR]:
            getTemplateSchemaError,
          [CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR]: createWorkspaceError,
        }}
        canCreateForUser={permissions?.createWorkspaceForUser}
        owner={owner}
        setOwner={(user) => {
          send({
            type: "SELECT_OWNER",
            owner: user,
          })
        }}
        onCancel={() => {
          // Go back
          navigate(-1)
        }}
        onSubmit={(request) => {
          send({
            type: "CREATE_WORKSPACE",
            request,
            owner,
          })
        }}
      />
    </>
  )
}

export default CreateWorkspacePage
