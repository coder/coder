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
import { FC } from "react"
import { Link as RouterLink } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { AvatarData } from "../../components/AvatarData/AvatarData"
import { EmptyState } from "../../components/EmptyState/EmptyState"
import { Stack } from "../../components/Stack/Stack"
import { TableLoader } from "../../components/TableLoader/TableLoader"
import { getDisplayStatus } from "../../util/workspace"

dayjs.extend(relativeTime)

export const Language = {
  createButton: "Create workspace",
  emptyMessage: "Create your first workspace",
  emptyDescription: "Start editing your source code and building your software",
}

export interface WorkspacesPageViewProps {
  loading?: boolean
  workspaces?: TypesGen.Workspace[]
}

export const WorkspacesPageView: FC<WorkspacesPageViewProps> = ({ loading, workspaces }) => {
  useStyles()
  const theme: Theme = useTheme()

  return (
    <Stack spacing={4}>
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
          {!workspaces && loading && <TableLoader />}
          {workspaces && workspaces.length === 0 && (
            <TableRow>
              <TableCell colSpan={999}>
                <EmptyState
                  message={Language.emptyMessage}
                  description={Language.emptyDescription}
                  cta={
                    <Link underline="none" component={RouterLink} to="/workspaces/new">
                      <Button startIcon={<AddCircleOutline />}>{Language.createButton}</Button>
                    </Link>
                  }
                />
              </TableCell>
            </TableRow>
          )}
          {workspaces &&
            workspaces.map((workspace) => {
              const status = getDisplayStatus(theme, workspace.latest_build)
              return (
                <TableRow key={workspace.id}>
                  <TableCell>
                    <AvatarData
                      title={workspace.name}
                      subtitle={workspace.owner_name}
                      link={`/@${workspace.owner_name}/${workspace.name}`}
                    />
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
    </Stack>
  )
}

const useStyles = makeStyles((theme) => ({
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
}))
