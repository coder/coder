import Table from "@mui/material/Table"
import TableBody from "@mui/material/TableBody"
import TableCell from "@mui/material/TableCell"
import TableContainer from "@mui/material/TableContainer"
import TableHead from "@mui/material/TableHead"
import TableRow from "@mui/material/TableRow"
import { Workspace } from "api/typesGenerated"
import { FC } from "react"
import { WorkspacesTableBody } from "./WorkspacesTableBody"

const Language = {
  name: "Name",
  template: "Template",
  lastUsed: "Last Used",
  status: "Status",
  lastBuiltBy: "Last Built By",
}

export interface WorkspacesTableProps {
  workspaces?: Workspace[]
  isUsingFilter: boolean
  onUpdateWorkspace: (workspace: Workspace) => void
  error?: Error | unknown
}

export const WorkspacesTable: FC<WorkspacesTableProps> = ({
  workspaces,
  isUsingFilter,
  onUpdateWorkspace,
  error,
}) => {
  return (
    <TableContainer>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell width="40%">{Language.name}</TableCell>
            <TableCell width="25%">{Language.template}</TableCell>
            <TableCell width="20%">{Language.lastUsed}</TableCell>
            <TableCell width="15%">{Language.status}</TableCell>
            <TableCell width="1%" />
          </TableRow>
        </TableHead>
        <TableBody>
          <WorkspacesTableBody
            workspaces={workspaces}
            isUsingFilter={isUsingFilter}
            onUpdateWorkspace={onUpdateWorkspace}
            error={error}
          />
        </TableBody>
      </Table>
    </TableContainer>
  )
}
