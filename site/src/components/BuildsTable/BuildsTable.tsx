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
import { BuildRow } from "./BuildRow"

export const Language = {
  emptyMessage: "No builds found",
}

export interface BuildsTableProps {
  builds?: TypesGen.WorkspaceBuild[]
}

export const BuildsTable: FC<React.PropsWithChildren<BuildsTableProps>> = ({
  builds,
}) => {
  return (
    <TableContainer>
      <Table data-testid="builds-table" aria-describedby="builds table">
        <TableBody>
          {builds ? (
            <Timeline
              items={builds}
              getDate={(build) => new Date(build.created_at)}
              row={(build) => <BuildRow key={build.id} build={build} />}
            />
          ) : (
            <TableLoader />
          )}

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
    </TableContainer>
  )
}
