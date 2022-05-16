import Avatar from "@material-ui/core/Avatar"
import Button from "@material-ui/core/Button"
import { makeStyles, Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import useTheme from "@material-ui/styles/useTheme"
import { useMachine } from "@xstate/react"
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import React from "react"
import { Link } from "react-router-dom"
import { WorkspaceBuild } from "../../api/typesGenerated"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { firstLetter } from "../../util/firstLetter"
import { workspacesMachine } from "../../xServices/workspaces/workspacesXService"

dayjs.extend(relativeTime)

export const Language = {
  title: "Workspaces",
}

export const WorkspacesPage: React.FC = () => {
  const styles = useStyles()
  const [workspacesState] = useMachine(workspacesMachine)
  const theme: Theme = useTheme()

  return (
    <Stack spacing={4}>
      <Margins>
        <div className={styles.actions}>
          <Button startIcon={<AddCircleOutline />}>Create Workspace</Button>
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
                  <div className={styles.workspaceName}>
                    <Avatar variant="square" className={styles.workspaceAvatar}>
                      {firstLetter(workspace.name)}
                    </Avatar>
                    <Link to={`/workspaces/${workspace.id}`} className={styles.workspaceLink}>
                      <b>{workspace.name}</b>
                      <span>{workspace.owner_name}</span>
                    </Link>
                  </div>
                </TableCell>
                <TableCell>{workspace.template_name}</TableCell>
                <TableCell>
                  {workspace.outdated ? (
                    <span style={{ color: theme.palette.error.main }}>outdated</span>
                  ) : (
                    <span style={{ color: theme.palette.text.secondary }}>up to date</span>
                  )}
                </TableCell>
                <TableCell>
                  <span style={{ color: theme.palette.text.secondary }}>
                    {dayjs().to(dayjs(workspace.latest_build.created_at))}
                  </span>
                </TableCell>
                <TableCell>{getStatus(theme, workspace.latest_build)}</TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      </Margins>
    </Stack>
  )
}

const getStatus = (theme: Theme, build: WorkspaceBuild): JSX.Element => {
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
    marginTop: theme.spacing(3),
    marginBottom: theme.spacing(3),
    display: "flex",
    height: theme.spacing(6),

    "& button": {
      marginLeft: "auto",
    },
  },
  workspaceRow: {
    "& > td": {
      paddingTop: theme.spacing(2),
      paddingBottom: theme.spacing(2),
    },
  },
  workspaceAvatar: {
    borderRadius: 2,
    marginRight: theme.spacing(1),
    width: 24,
    height: 24,
    fontSize: 16,
  },
  workspaceName: {
    display: "flex",
    alignItems: "center",
  },
  workspaceLink: {
    display: "flex",
    flexDirection: "column",
    color: theme.palette.text.primary,
    textDecoration: "none",
    "&:hover": {
      textDecoration: "underline",
    },
    "& span": {
      fontSize: 12,
      color: theme.palette.text.secondary,
    },
  },
}))
