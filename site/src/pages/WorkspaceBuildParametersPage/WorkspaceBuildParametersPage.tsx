import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useTranslation } from "react-i18next"
import { pageTitle } from "util/page"
import { useMachine } from "@xstate/react"
import { useNavigate, useParams } from "react-router-dom"
import { workspaceBuildParametersMachine } from "xServices/workspace/workspaceBuildParametersXService"
import { WorkspaceBuildParametersPageView } from "./WorkspaceBuildParametersPageView"

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
    selectedTemplate,
    templateParameters,
    buildParameters,
    updateWorkspaceError,
  } = state.context

  return (
    <>
      <Helmet>
        <title>{pageTitle(t("title"))}</title>
      </Helmet>
      <WorkspaceBuildParametersPageView isLoading={false} />
    </>
  )
}
