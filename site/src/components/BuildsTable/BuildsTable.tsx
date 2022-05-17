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
  pageTitle: "Builds",
  usersTitle: "All users",
  emptyMessage: "No users found",
  usernameLabel: "User",
  suspendMenuItem: "Suspend",
  resetPasswordMenuItem: "Reset password",
  rolesLabel: "Roles",
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

            let displayDuration = Language.inProgressLabel
            if (b.job.started_at && b.job.completed_at) {
              const startedAt = dayjs(b.job.started_at)
              const completedAt = dayjs(b.job.completed_at)
              const diff = completedAt.diff(startedAt, "seconds")
              displayDuration = `${diff} seconds`
            }

            return (
              <TableRow key={b.id}>
                <TableCell>{b.transition}</TableCell>
                <TableCell>
                  <span style={{ color: theme.palette.text.secondary }}>{displayDuration}</span>
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
                <EmptyState message="No builds for this workspace" />
              </Box>
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  )
}
