import Box from "@material-ui/core/Box"
import { Theme } from "@material-ui/core/styles"
import Table from "@material-ui/core/Table"
import TableBody from "@material-ui/core/TableBody"
import TableCell from "@material-ui/core/TableCell"
import TableContainer from "@material-ui/core/TableContainer"
import TableHead from "@material-ui/core/TableHead"
import TableRow from "@material-ui/core/TableRow"
import useTheme from "@material-ui/styles/useTheme"
import { FC } from "react"
import * as TypesGen from "../../api/typesGenerated"
import { EmptyState } from "../EmptyState/EmptyState"
import { TableLoader } from "../TableLoader/TableLoader"

export const Language = {
  emptyMessage: "No versions found",
  nameLabel: "Version name",
  createdAtLabel: "Created at",
  createdByLabel: "Created by",
}

export interface VersionsTableProps {
  versions?: TypesGen.TemplateVersion[]
}

export const VersionsTable: FC<React.PropsWithChildren<VersionsTableProps>> = ({ versions }) => {
  const isLoading = !versions
  const theme: Theme = useTheme()

  return (
    <TableContainer>
      <Table data-testid="versions-table">
        <TableHead>
          <TableRow>
            <TableCell width="30%">{Language.nameLabel}</TableCell>
            <TableCell width="30%">{Language.createdAtLabel}</TableCell>
            <TableCell width="40%">{Language.createdByLabel}</TableCell>
          </TableRow>
        </TableHead>
        <TableBody>
          {isLoading && <TableLoader />}
          {versions &&
            versions
              .slice()
              .reverse()
              .map((version) => {
                return (
                  <TableRow key={version.id} data-testid={`version-${version.id}`}>
                    <TableCell>{version.name}</TableCell>
                    <TableCell>
                      <span style={{ color: theme.palette.text.secondary }}>
                        {new Date(version.created_at).toLocaleString()}
                      </span>
                    </TableCell>
                    <TableCell>
                      <span style={{ color: theme.palette.text.secondary }}>
                        {version.created_by_name}
                      </span>
                    </TableCell>
                  </TableRow>
                )
              })}

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
