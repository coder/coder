import Box from "@mui/material/Box"
import Table from "@mui/material/Table"
import TableBody from "@mui/material/TableBody"
import TableCell from "@mui/material/TableCell"
import TableContainer from "@mui/material/TableContainer"
import TableRow from "@mui/material/TableRow"
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
  activeVersionId: string
  onPromoteClick?: (templateVersionId: string) => void
  versions?: TypesGen.TemplateVersion[]
}

export const VersionsTable: FC<React.PropsWithChildren<VersionsTableProps>> = ({
  versions,
  onPromoteClick,
  activeVersionId,
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
                <VersionRow
                  onPromoteClick={onPromoteClick}
                  version={version}
                  key={version.id}
                  isActive={activeVersionId === version.id}
                />
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
