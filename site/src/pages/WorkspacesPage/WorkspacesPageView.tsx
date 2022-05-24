import Avatar from "@material-ui/core/Avatar"
import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
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
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { firstLetter } from "../../util/firstLetter"
import { getDisplayStatus } from "../../util/workspace"

dayjs.extend(relativeTime)

export const Language = {
  createButton: "Create workspace",
  emptyView: "so you can check out your repositories, edit your source code, and build and test your software.",
}

export interface WorkspacesPageViewProps {
  loading?: boolean
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
          <Link underline="none" component={RouterLink} to="/workspaces/new">
            <Button startIcon={<AddCircleOutline />}>{Language.createButton}</Button>
          </Link>
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
            {!props.loading && !props.workspaces?.length && (
              <TableRow>
                <TableCell colSpan={999}>
                  <div className={styles.welcome}>
                    <span>
                      <Link component={RouterLink} to="/templates">
                        Create a workspace
                      </Link>
                      &nbsp;{Language.emptyView}
                    </span>
                  </div>
                </TableCell>
              </TableRow>
            )}
            {props.workspaces?.map((workspace) => {
              const status = getDisplayStatus(theme, workspace.latest_build)
              return (
                <TableRow key={workspace.id}>
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
                    <span data-chromatic="ignore" style={{ color: theme.palette.text.secondary }}>
                      {dayjs().to(dayjs(workspace.latest_build.created_at))}
                    </span>
                  </TableCell>
                  <TableCell>
                    <span style={{ color: status.color }}>{status.status}</span>
                  </TableCell>
                </TableRow>
              )
            })}
          </TableBody>
        </Table>
      </Margins>
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
  actions: {
    marginTop: theme.spacing(3),
    marginBottom: theme.spacing(3),
    display: "flex",
    height: theme.spacing(6),

    "& > *": {
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
