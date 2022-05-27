import Button from "@material-ui/core/Button"
import Link from "@material-ui/core/Link"
import { makeStyles, Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import TextField from "@material-ui/core/TextField/TextField"
import AddCircleOutline from "@material-ui/icons/AddCircleOutline"
import useTheme from "@material-ui/styles/useTheme"
import dayjs from "dayjs"
import relativeTime from "dayjs/plugin/relativeTime"
import { Link as RouterLink } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { AvatarData } from "../../components/AvatarData/AvatarData"
import { Margins } from "../../components/Margins/Margins"
import { Stack } from "../../components/Stack/Stack"
import { getDisplayStatus } from "../../util/workspace"
import React, { useCallback, useState } from "react"

dayjs.extend(relativeTime)

export const Language = {
  filterLabel: "Filter",
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
  const [filteredWorkspaces, setFilteredWorkspaces] = useState<TypesGen.Workspace[] | undefined>(props.workspaces)
  const [query, setQuery] = useState<string>("owner:f0ssel")
  const handleQueryChange = useCallback(() => {
      setQuery((query) => query)

      if (query.length && filteredWorkspaces?.length) {
        const owners: string[] = []
        const newWorkspacers: TypesGen.Workspace[] = []
        const phrases = query.split(" ")
        for (const p of phrases) {
          if (p.startsWith("owner:")) {
            owners.push(p.slice("owner:".length))
          }
        }

        for (const w of filteredWorkspaces) {
          for (const o of owners) {
            if (o === w.owner_name) {
              newWorkspacers.push(w)
            }
          }
        }
        setFilteredWorkspaces(newWorkspacers)
      }
  }, [query, filteredWorkspaces])

  return (
    <Stack spacing={4}>
      <Margins>
        <div className={styles.actions}>
          <TextField
            onChange={handleQueryChange}
            value={query}
          />
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
            {!props.loading && !filteredWorkspaces?.length && (
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
            {filteredWorkspaces?.map((workspace) => {
              const status = getDisplayStatus(theme, workspace.latest_build)
              return (
                <TableRow key={workspace.id}>
                  <TableCell>
                    <AvatarData
                      title={workspace.name}
                      subtitle={workspace.owner_name}
                      link={`/workspaces/${workspace.id}`}
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
}))
