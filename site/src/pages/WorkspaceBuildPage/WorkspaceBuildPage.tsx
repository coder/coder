import { useMachine } from "@xstate/react"
import { FC } from "react"
import { Helmet } from "react-helmet-async"
import { useParams } from "react-router-dom"
import { pageTitle } from "../../utils/page"
import { workspaceBuildMachine } from "../../xServices/workspaceBuild/workspaceBuildXService"
import { WorkspaceBuildPageView } from "./WorkspaceBuildPageView"

export const WorkspaceBuildPage: FC = () => {
  const params = useParams() as {
    username: string
    workspace: string
    buildNumber: string
  }
  const workspaceName = params.workspace
  const buildNumber = params.buildNumber
  const username = params.username.replace("@", "")
  const [buildState] = useMachine(workspaceBuildMachine, {
    context: { username, workspaceName, buildNumber, timeCursor: new Date() },
  })
  const { logs, build } = buildState.context

  return (
    <>
      <Helmet>
        <title>
          {build
            ? pageTitle(
                `Build #${build.build_number} · ${build.workspace_name}`,
              )
            : ""}
        </title>
      </Helmet>

      <WorkspaceBuildPageView logs={logs} build={build} />
    </>
  )
}

export default WorkspaceBuildPage
