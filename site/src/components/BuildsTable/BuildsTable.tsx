import Box from "@material-ui/core/Box"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import React from "react"
import * as TypesGen from "../../api/typesGenerated"
import { EmptyState } from "../EmptyState/EmptyState"
import { TableHeaderRow } from "../TableHeaders/TableHeaders"
import { TableLoader } from "../TableLoader/TableLoader"

export const Language = {
  pageTitle: "Builds",
  usersTitle: "All users",
  emptyMessage: "No users found",
  usernameLabel: "User",
  suspendMenuItem: "Suspend",
  resetPasswordMenuItem: "Reset password",
  rolesLabel: "Roles",
}

export interface BuildsTableProps {
  builds?: TypesGen.WorkspaceBuild[]
}

export const BuildsTable: React.FC<BuildsTableProps> = ({ builds }) => {
  const isLoading = !builds

  return (
    <Table>
      <TableHead>
        <TableHeaderRow>
          <TableCell size="small">Action</TableCell>
          <TableCell size="small">Duration</TableCell>
          <TableCell size="small">Started at</TableCell>
          <TableCell size="small">Status</TableCell>
        </TableHeaderRow>
      </TableHead>
      <TableBody>
        {isLoading && <TableLoader />}
        {builds &&
          builds.map((b) => (
            <TableRow key={b.id}>
              <TableCell>{b.transition}</TableCell>
              <TableCell>{b.created_at}</TableCell>
              <TableCell>{b.created_at}</TableCell>
              <TableCell>{b.job.status}</TableCell>
            </TableRow>
          ))}

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
