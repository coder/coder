import { useMachine } from "@xstate/react"
import { TemplateVersionParameter } from "api/typesGenerated"
import { useMe } from "hooks/useMe"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useParams, useSearchParams } from "react-router-dom"
import { pageTitle } from "util/page"
import { createWorkspaceMachine } from "xServices/createWorkspace/createWorkspaceXService"
import {
  CreateWorkspaceErrors,
  CreateWorkspacePageView,
} from "./CreateWorkspacePageView"

const CreateWorkspacePage: FC = () => {
  const organizationId = useOrganizationId()
  const { template: templateName } = useParams() as { template: string }
  const navigate = useNavigate()
  const me = useMe()
  const [createWorkspaceState, send] = useMachine(createWorkspaceMachine, {
    context: {
      organizationId,
      templateName,
      owner: me,
    },
    actions: {
      onCreateWorkspace: (_, event) => {
        navigate(`/@${event.data.owner_name}/${event.data.name}`)
      },
    },
  })
  const {
    templates,
    templateParameters,
    templateSchema,
    selectedTemplate,
    getTemplateSchemaError,
    getTemplatesError,
    createWorkspaceError,
    permissions,
    owner,
  } = createWorkspaceState.context
  const [searchParams] = useSearchParams()
  const defaultParameterValues = getDefaultParameterValues(searchParams)

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Workspace")}</title>
      </Helmet>
      <CreateWorkspacePageView
        defaultParameterValues={defaultParameterValues}
        loadingTemplates={createWorkspaceState.matches("gettingTemplates")}
        loadingTemplateSchema={createWorkspaceState.matches(
          "gettingTemplateSchema",
        )}
        creatingWorkspace={createWorkspaceState.matches("creatingWorkspace")}
        hasTemplateErrors={createWorkspaceState.matches("error")}
        templateName={templateName}
        templates={templates}
        selectedTemplate={selectedTemplate}
        templateParameters={orderedTemplateParameters(templateParameters)}
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

const getDefaultParameterValues = (
  urlSearchParams: URLSearchParams,
): Record<string, string> => {
  const paramValues: Record<string, string> = {}
  Array.from(urlSearchParams.keys())
    .filter((key) => key.startsWith("param."))
    .forEach((key) => {
      const paramName = key.replace("param.", "")
      const paramValue = urlSearchParams.get(key)
      paramValues[paramName] = paramValue ?? ""
    })
  return paramValues
}

export const orderedTemplateParameters = (
  templateParameters?: TemplateVersionParameter[],
): TemplateVersionParameter[] => {
  if (!templateParameters) {
    return []
  }

  const immutables = templateParameters.filter(
    (parameter) => !parameter.mutable,
  )
  const mutables = templateParameters.filter((parameter) => parameter.mutable)
  return [...immutables, ...mutables]
}

export default CreateWorkspacePage
