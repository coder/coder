import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import { FC } from "react"
import { WorkspaceItemMachineRef } from "../../xServices/workspaces/workspacesXService"
import { WorkspacesTableBody } from "./WorkspacesTableBody"

const Language = {
  name: "Name",
  template: "Template",
  version: "Version",
  lastBuilt: "Last Built",
  status: "Status",
}

export interface WorkspacesTableProps {
  isLoading?: boolean
  workspaceRefs?: WorkspaceItemMachineRef[]
  filter?: string
}

export const WorkspacesTable: FC<WorkspacesTableProps> = ({ isLoading, workspaceRefs, filter }) => {
  return (
    <Table>
      <TableHead>
        <TableRow>
          <TableCell width="35%">{Language.name}</TableCell>
          <TableCell width="15%">{Language.template}</TableCell>
          <TableCell width="15%">{Language.version}</TableCell>
          <TableCell width="20%">{Language.lastBuilt}</TableCell>
          <TableCell width="15%">{Language.status}</TableCell>
          <TableCell width="1%"></TableCell>
        </TableRow>
      </TableHead>
      <TableBody>
        <WorkspacesTableBody isLoading={isLoading} workspaceRefs={workspaceRefs} filter={filter} />
      </TableBody>
    </Table>
  )
}
