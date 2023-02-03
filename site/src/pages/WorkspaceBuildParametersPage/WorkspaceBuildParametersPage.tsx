import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { pageTitle } from "util/page"
import { useMachine } from "@xstate/react"
import { useNavigate, useParams } from "react-router-dom"
import { workspaceBuildParametersMachine } from "xServices/workspace/workspaceBuildParametersXService"
import {
  UpdateWorkspaceErrors,
  WorkspaceBuildParametersPageView,
} from "./WorkspaceBuildParametersPageView"
import { orderedTemplateParameters } from "pages/CreateWorkspacePage/CreateWorkspacePage"

export const WorkspaceBuildParametersPage: FC = () => {
  const { t } = useTranslation("workspaceBuildParametersPage")

  const navigate = useNavigate()
  const { owner: workspaceOwner, workspace: workspaceName } = useParams() as {
    owner: string
    workspace: string
  }
  const [state, send] = useMachine(workspaceBuildParametersMachine, {
    context: {
      workspaceOwner,
      workspaceName,
    },
    actions: {
      onUpdateWorkspace: (_, event) => {
        navigate(
          `/@${event.data.workspace_owner_name}/${event.data.workspace_name}`,
        )
      },
    },
  })
  const {
    selectedWorkspace,
    templateParameters,
    workspaceBuildParameters,
    getWorkspaceError,
    getTemplateParametersError,
    getWorkspaceBuildParametersError,
    updateWorkspaceError,
  } = state.context

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>
      <WorkspaceBuildParametersPageView
        workspace={selectedWorkspace}
        templateParameters={orderedTemplateParameters(templateParameters)}
        workspaceBuildParameters={workspaceBuildParameters}
        isLoading={
          state.matches("gettingWorkspace") ||
          state.matches("gettingTemplateParameters") ||
          state.matches("gettingWorkspaceBuildParameters")
        }
        updatingWorkspace={state.matches("updatingWorkspace")}
        hasErrors={state.matches("error")}
        updateWorkspaceErrors={{
          [UpdateWorkspaceErrors.GET_WORKSPACE_ERROR]: getWorkspaceError,
          [UpdateWorkspaceErrors.GET_TEMPLATE_PARAMETERS_ERROR]:
            getTemplateParametersError,
          [UpdateWorkspaceErrors.GET_WORKSPACE_BUILD_PARAMETERS_ERROR]:
            getWorkspaceBuildParametersError,
          [UpdateWorkspaceErrors.UPDATE_WORKSPACE_ERROR]: updateWorkspaceError,
        }}
        onCancel={() => {
          // Go back
          navigate(-1)
        }}
        onSubmit={(request) => {
          send({
            type: "UPDATE_WORKSPACE",
            request,
          })
        }}
      />
    </>
  )
}
