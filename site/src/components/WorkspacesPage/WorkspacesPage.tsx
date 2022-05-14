import { TextField } from "@material-ui/core"
import Button from "@material-ui/core/Button"
import { makeStyles } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import useTheme from "@material-ui/styles/useTheme"
import { useMachine } from "@xstate/react"
import React from "react"
import { WorkspaceBuild } from "../../api/typesGenerated"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { getTimeSince } from "../../util/time"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"

export const Language = {
  title: "Workspaces",
}

export const WorkspacesPage: React.FC = () => {
  const styles = useStyles()
  const [workspacesState] = useMachine(workspacesMachine)
  const theme = useTheme()

  return (
    <Stack spacing={4}>
      <Margins>
        <img className={styles.boxes} alt="boxes" src="/boxes.png" />
        <div className={styles.actions}>
          <TextField placeholder="Search all workspaces" />
          <Button color="primary">Create Workspace</Button>
        </div>
        <Table>
          <TableHead>
            <TableRow>
              <TableCell>Name</TableCell>
              <TableCell>Template</TableCell>
              <TableCell>Version</TableCell>
              <TableCell>Last Built</TableCell>
              <TableCell>Status</TableCell>
            </TableRow>
          </TableHead>
          <TableBody>
            {workspacesState.context.workspaces?.map((workspace) => (
              <TableRow key={workspace.id} className={styles.workspaceRow}>
                <TableCell>
                  <b>{workspace.name}</b>
                </TableCell>
                <TableCell>{workspace.template_name}</TableCell>
                <TableCell>{workspace.latest_build.template_version_id}</TableCell>
                <TableCell>{getTimeSince(new Date(workspace.latest_build.created_at))} ago</TableCell>
                <TableCell>{getStatus(theme, workspace.latest_build)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Margins>
    </Stack>
  )
}

const getStatus = (theme: any, build: WorkspaceBuild): JSX.Element => {
  let status = ""
  let color = ""
  const inProgress = build.job.status === "running" || build.job.status === "canceling"

  switch (build.transition) {
    case "start":
      color = theme.palette.success.main
      status = inProgress ? "⦿ Starting" : "⦿ Running"
      break
    case "stop":
      color = theme.palette.text.secondary
      status = inProgress ? "◍ Stopping" : "◍ Stopped"
      break
    case "delete":
      status = inProgress ? "Deleting" : "Deleted"
      break
  }
  if (build.job.status === "failed") {
    color = theme.palette.error.main
    status = "ⓧ Failed"
  }

  return <span style={{ color: color }}>{status}</span>
}

const useStyles = makeStyles((theme) => ({
  actions: {
    marginTop: theme.spacing(6),
    marginBottom: theme.spacing(4),
  },
  boxes: {
    position: "absolute",
    pointerEvents: "none",
    top: "0%",
    left: "50%",
    transform: "translateX(-50%)",
    zIndex: -1,
  },
  workspaceRow: {
    "& > td": {
      paddingTop: theme.spacing(2),
      paddingBottom: theme.spacing(2),
    },
  },
}))
