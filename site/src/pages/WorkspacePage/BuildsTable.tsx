import Box from "@mui/material/Box";
import Table from "@mui/material/Table";
import TableBody from "@mui/material/TableBody";
import TableCell from "@mui/material/TableCell";
import TableContainer from "@mui/material/TableContainer";
import TableRow from "@mui/material/TableRow";
import { Timeline } from "components/Timeline/Timeline";
import { FC } from "react";
import * as TypesGen from "api/typesGenerated";
import { EmptyState } from "components/EmptyState/EmptyState";
import { TableLoader } from "components/TableLoader/TableLoader";
import { BuildRow } from "./BuildRow";

export const Language = {
  emptyMessage: "No builds found",
};

export interface BuildsTableProps {
  builds?: TypesGen.WorkspaceBuild[];
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
  );
};
