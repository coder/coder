import Box from "@material-ui/core/Box"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableRow from "@material-ui/core/TableRow"
import { Timeline } from "components/Timeline/Timeline"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { EmptyState } from "../EmptyState/EmptyState"
import { TableLoader } from "../TableLoader/TableLoader"
import { VersionRow } from "./VersionRow"

export const Language = {
  emptyMessage: "No versions found",
  nameLabel: "Version name",
  createdAtLabel: "Created at",
  createdByLabel: "Created by",
}

export interface VersionsTableProps {
  versions?: TypesGen.TemplateVersion[]
}

export const VersionsTable: FC<React.PropsWithChildren<VersionsTableProps>> = ({
  versions,
}) => {
  return (
    <TableContainer>
      <Table data-testid="versions-table">
        <TableBody>
          {versions ? (
            <Timeline
              items={versions.slice().reverse()}
              getDate={(version) => new Date(version.created_at)}
              row={(version) => (
                <VersionRow version={version} key={version.id} />
              )}
            />
          ) : (
            <TableLoader />
          )}

          {versions && versions.length === 0 && (
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
    </TableContainer>
  )
}
