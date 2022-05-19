import Box from "@material-ui/core/Box"
import { makeStyles, Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import useTheme from "@material-ui/styles/useTheme"
import React from "react"
import { useNavigate } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { displayWorkspaceBuildDuration, getDisplayStatus } from "../../util/workspace"
import { EmptyState } from "../EmptyState/EmptyState"
import { TableLoader } from "../TableLoader/TableLoader"

export const Language = {
  emptyMessage: "No builds found",
  inProgressLabel: "In progress",
  actionLabel: "Action",
  durationLabel: "Duration",
  startedAtLabel: "Started at",
  statusLabel: "Status",
}

export interface BuildsTableProps {
  builds?: TypesGen.WorkspaceBuild[]
  className?: string
}

export const BuildsTable: React.FC<BuildsTableProps> = ({ builds, className }) => {
  const isLoading = !builds
  const theme: Theme = useTheme()
  const navigate = useNavigate()
  const styles = useStyles()

  return (
    <Table className={className}>
      <TableHead>
        <TableRow>
          <TableCell width="20%">{Language.actionLabel}</TableCell>
          <TableCell width="20%">{Language.durationLabel}</TableCell>
          <TableCell width="40%">{Language.startedAtLabel}</TableCell>
          <TableCell width="20%">{Language.statusLabel}</TableCell>
        </TableRow>
      </TableHead>
      <TableBody>
        {isLoading && <TableLoader />}
        {builds &&
          builds.map((b) => {
            const status = getDisplayStatus(theme, b)

            const navigateToBuildPage = () => {
              navigate(`/builds/${b.id}`)
            }

            return (
              <TableRow
                hover
                key={b.id}
                data-testid={`build-${b.id}`}
                tabIndex={0}
                onClick={navigateToBuildPage}
                onKeyDown={(event) => {
                  if (event.key === "Enter") {
                    navigateToBuildPage()
                  }
                }}
                className={styles.clickableTableRow}
              >
                <TableCell>{b.transition}</TableCell>
                <TableCell>
                  <span style={{ color: theme.palette.text.secondary }}>{displayWorkspaceBuildDuration(b)}</span>
                </TableCell>
                <TableCell>
                  <span style={{ color: theme.palette.text.secondary }}>{new Date(b.created_at).toLocaleString()}</span>
                </TableCell>
                <TableCell>
                  <span style={{ color: status.color }}>{status.status}</span>
                </TableCell>
              </TableRow>
            )
          })}

        {builds && builds.length === 0 && (
          <TableRow>
            <TableCell colSpan={999}>
              <Box p={4}>
                <EmptyState message={Language.emptyMessage} />
              </Box>
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  )
}

const useStyles = makeStyles((theme) => ({
  clickableTableRow: {
    cursor: "pointer",

    "&:hover td": {
      backgroundColor: theme.palette.background.default,
    },

    "&:focus": {
      outline: `1px solid ${theme.palette.primary.dark}`,
    },
  },
}))
