import { Link } from "@material-ui/core"
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
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import React from "react"
import { Link as RouterLink } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { WorkspaceBuild } from "../../api/typesGenerated"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { firstLetter } from "../../util/firstLetter"

dayjs.extend(relativeTime)

export const Language = {
  title: "Workspaces",
}

export interface WorkspacesPageViewProps {
  workspaces?: TypesGen.Workspace[]
  error?: unknown
}

export const WorkspacesPageView: React.FC<WorkspacesPageViewProps> = (props) => {
  const styles = useStyles()
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
            {!props.workspaces && (
              <TableRow>
                <TableCell colSpan={999}>
                  <div className={styles.welcome}>
                    <span>
                      <Link component={RouterLink} to="/workspaces/new">
                        Create a workspace
                      </Link>
                      &nbsp;so you can check out your repositories, edit your source code, and build and test your software.
                    </span>
                  </div>
                </TableCell>
              </TableRow>
            )}
            {props.workspaces?.map((workspace) => (
              <TableRow key={workspace.id} className={styles.workspaceRow}>
                <TableCell>
                  <div className={styles.workspaceName}>
                    <Avatar variant="square" className={styles.workspaceAvatar}>
                      {firstLetter(workspace.name)}
                    </Avatar>
                    <Link component={RouterLink} to={`/workspaces/${workspace.id}`} className={styles.workspaceLink}>
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
  const inProgress = build.job.status === "running"
  switch (build.job.status) {
    case "running":
    case "succeeded":
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
          color = theme.palette.text.secondary
          status = inProgress ? "⦸ Deleting" : "⦸ Deleted"
          break
      }
      break
    case "canceled":
      color = theme.palette.text.secondary
      status = "◍ Canceled"
      break
    case "canceling":
      color = theme.palette.warning.main
      status = "◍ Canceling"
      break
    case "failed":
      color = theme.palette.error.main
      status = "ⓧ Failed"
      break
    case "pending":
      color = theme.palette.text.secondary
      status = "◍ Queued"
      break
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
  welcome: {
    paddingTop: theme.spacing(12),
    paddingBottom: theme.spacing(12),
    display: "flex",
    flexDirection: "column",
    alignItems: "center",
    justifyContent: "center",
    "& span": {
      maxWidth: 600,
      textAlign: "center",
      fontSize: theme.spacing(2),
      lineHeight: `${theme.spacing(3)}px`,
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
