import { makeStyles } from "@material-ui/core/styles"
import Typography from "@material-ui/core/Typography"
import { useMachine } from "@xstate/react"
import React from "react"
import { useParams } from "react-router-dom"
import { Loader } from "../../components/Loader/Loader"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { WorkspaceBuildLogs } from "../../components/WorkspaceBuildLogs/WorkspaceBuildLogs"
import { WorkspaceBuildStats } from "../../components/WorkspaceBuildStats/WorkspaceBuildStats"
import { workspaceBuildMachine } from "../../xServices/workspaceBuild/workspaceBuildXService"

const useBuildId = () => {
  const { buildId } = useParams()

  if (!buildId) {
    throw new Error("buildId param is required.")
  }

  return buildId
}

export const WorkspaceBuildPage: React.FC = () => {
  const buildId = useBuildId()
  const [buildState] = useMachine(workspaceBuildMachine, { context: { buildId } })
  const { logs, build } = buildState.context
  const styles = useStyles()

  return (
    <Margins>
      <Stack>
        <Typography variant="h4" className={styles.title}>
          Logs
        </Typography>

        {build && <WorkspaceBuildStats build={build} />}
        {!logs && <Loader />}
        {logs && <WorkspaceBuildLogs logs={logs} />}
      </Stack>
    </Margins>
  )
}

const useStyles = makeStyles((theme) => ({
  title: {
    marginTop: theme.spacing(5),
  },
}))
