import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useNavigate, useParams } from "react-router-dom"
import { changeWorkspaceVersionMachine } from "xServices/workspace/changeWorkspaceVersionXService"
import { WorkspaceChangeVersionPageView } from "./WorkspaceChangeVersionPageView"

export const WorkspaceChangeVersionPage: FC = () => {
  const navigate = useNavigate()
  const { username: owner, workspace: workspaceName } = useParams() as {
    username: string
    workspace: string
  }
  const [state, send] = useMachine(changeWorkspaceVersionMachine, {
    context: {
      owner,
      workspaceName,
    },
    actions: {
      onUpdateVersion: () => {
        navigate(-1)
      },
    },
  })

  return (
    <>
      <Helmet>
        <title>{`Change version · ${workspaceName}`}</title>
      </Helmet>

      <WorkspaceChangeVersionPageView
        isUpdating={state.matches("updatingVersion")}
        context={state.context}
        onSubmit={(versionId) => {
          send({
            type: "UPDATE_VERSION",
            versionId,
          })
        }}
      />
    </>
  )
}

export default WorkspaceChangeVersionPage
