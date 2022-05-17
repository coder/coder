import Box from "@material-ui/core/Box"
import { Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import useTheme from "@material-ui/styles/useTheme"
import dayjs from "dayjs"
import duration from "dayjs/plugin/duration"
import relativeTime from "dayjs/plugin/relativeTime"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { getDisplayStatus } from "../../util/workspace"
import { EmptyState } from "../EmptyState/EmptyState"
import { TableLoader } from "../TableLoader/TableLoader"

dayjs.extend(relativeTime)
dayjs.extend(duration)

export const Language = {
  emptyMessage: "No builds found",
  inProgressLabel: "In progress",
  actionLabel: "Action",
  durationLabel: "Duration",
  startedAtLabel: "Started at",
  statusLabel: "Status",
}

const getDurationInSeconds = (build: TypesGen.WorkspaceBuild) => {
  let display = Language.inProgressLabel

  if (build.job.started_at && build.job.completed_at) {
    const startedAt = dayjs(build.job.started_at)
    const completedAt = dayjs(build.job.completed_at)
    const diff = completedAt.diff(startedAt, "seconds")
    display = `${diff} seconds`
  }

  return display
}

export interface BuildsTableProps {
  builds?: TypesGen.WorkspaceBuild[]
  className?: string
}

export const BuildsTable: React.FC<BuildsTableProps> = ({ builds, className }) => {
  const isLoading = !builds
  const theme: Theme = useTheme()

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
            const duration = getDurationInSeconds(b)

            return (
              <TableRow key={b.id} data-testid={`build-${b.id}`}>
                <TableCell>{b.transition}</TableCell>
                <TableCell>
                  <span style={{ color: theme.palette.text.secondary }}>{duration}</span>
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
