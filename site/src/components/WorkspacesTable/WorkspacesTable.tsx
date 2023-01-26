import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { Workspace } from "api/typesGenerated"
import { FC } from "react"
import { WorkspacesTableBody } from "./WorkspacesTableBody"

const Language = {
  name: "Name",
  template: "Template",
  lastUsed: "Last Used",
  version: "Version",
  status: "Status",
  lastBuiltBy: "Last Built By",
}

export interface WorkspacesTableProps {
  workspaces?: Workspace[]
  isUsingFilter: boolean
  onUpdateWorkspace: (workspace: Workspace) => void
}

export const WorkspacesTable: FC<WorkspacesTableProps> = ({
  workspaces,
  isUsingFilter,
  onUpdateWorkspace,
}) => {
  return (
    <TableContainer>
      <Table>
        <TableHead>
          <TableRow>
            <TableCell width="30%">{Language.name}</TableCell>
            <TableCell width="25%">{Language.template}</TableCell>
            <TableCell width="25%">{Language.version}</TableCell>
            <TableCell width="20%">{Language.lastUsed}</TableCell>
            <TableCell width="20%">{Language.status}</TableCell>
            <TableCell width="1%"></TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          <WorkspacesTableBody
            workspaces={workspaces}
            isUsingFilter={isUsingFilter}
            onUpdateWorkspace={onUpdateWorkspace}
          />
        </TableBody>
      </Table>
    </TableContainer>
  )
}
