import { useMachine } from "@xstate/react"
import { TemplateVersionParameter } from "api/typesGenerated"
import { useMe } from "hooks/useMe"
import { useOrganizationId } from "hooks/useOrganizationId"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useParams, useSearchParams } from "react-router-dom"
import { pageTitle } from "utils/page"
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
    templateGitAuth,
    selectedTemplate,
    getTemplateGitAuthError,
    getTemplatesError,
    createWorkspaceError,
    permissions,
    owner,
  } = createWorkspaceState.context
  const [searchParams] = useSearchParams()
  const defaultParameterValues = getDefaultParameterValues(searchParams)
  const name = getName(searchParams)

  return (
    <>
      <Helmet>
        <title>{pageTitle("Create Workspace")}</title>
      </Helmet>
      <CreateWorkspacePageView
        name={name}
        defaultParameterValues={defaultParameterValues}
        loadingTemplates={createWorkspaceState.matches("gettingTemplates")}
        creatingWorkspace={createWorkspaceState.matches("creatingWorkspace")}
        hasTemplateErrors={createWorkspaceState.matches("error")}
        templateName={templateName}
        templates={templates}
        selectedTemplate={selectedTemplate}
        templateParameters={orderedTemplateParameters(templateParameters)}
        templateGitAuth={templateGitAuth}
        createWorkspaceErrors={{
          [CreateWorkspaceErrors.GET_TEMPLATES_ERROR]: getTemplatesError,
          [CreateWorkspaceErrors.CREATE_WORKSPACE_ERROR]: createWorkspaceError,
          [CreateWorkspaceErrors.GET_TEMPLATE_GITAUTH_ERROR]:
            getTemplateGitAuthError,
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

const getName = (urlSearchParams: URLSearchParams): string => {
  return urlSearchParams.get("name") ?? ""
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
