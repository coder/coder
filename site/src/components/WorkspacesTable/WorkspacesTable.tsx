import Box from "@material-ui/core/Box"
import Button from "@material-ui/core/Button"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import React from "react"
import { Link } from "react-router-dom"
import * as TypesGen from "../../api/typesGenerated"
import { EmptyState } from "../EmptyState/EmptyState"
import { TableHeaderRow } from "../TableHeaders/TableHeaders"
import { TableLoader } from "../TableLoader/TableLoader"
import { TableTitle } from "../TableTitle/TableTitle"

export const Language = {
  title: "Workspaces",
  nameLabel: "Name",
  emptyMessage: "No workspaces have been created yet",
  emptyDescription: "Create a workspace to get started",
  ctaAction: "Create workspace",
}

export interface WorkspacesTableProps {
  templateInfo?: TypesGen.Template
  workspaces?: TypesGen.Workspace[]
  onCreateWorkspace: () => void
}

export const WorkspacesTable: React.FC<WorkspacesTableProps> = ({ templateInfo, workspaces, onCreateWorkspace }) => {
  const isLoading = !templateInfo || !workspaces

  return (
    <Table>
      <TableHead>
        <TableTitle title={Language.title} />
        <TableHeaderRow>
          <TableCell size="small">{Language.nameLabel}</TableCell>
        </TableHeaderRow>
      </TableHead>
      <TableBody>
        {isLoading && <TableLoader />}
        {workspaces &&
          workspaces.map((w) => (
            <TableRow key={w.id}>
              <TableCell>
                <Link to={`/workspaces/${w.id}`}>{w.name}</Link>
              </TableCell>
            </TableRow>
          ))}

        {workspaces && workspaces.length === 0 && (
          <TableRow>
            <TableCell colSpan={999}>
              <Box p={4}>
                <EmptyState
                  message={Language.emptyMessage}
                  description={Language.emptyDescription}
                  cta={
                    <Button variant="contained" color="primary" onClick={onCreateWorkspace}>
                      {Language.ctaAction}
                    </Button>
                  }
                />
              </Box>
            </TableCell>
          </TableRow>
        )}
      </TableBody>
    </Table>
  )
}
