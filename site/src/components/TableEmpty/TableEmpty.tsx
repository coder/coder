import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import { FC } from "react";
import { EmptyState, EmptyStateProps } from "components/EmptyState/EmptyState";

export type TableEmptyProps = EmptyStateProps;

export const TableEmpty: FC<TableEmptyProps> = (props) => {
  return (
    <TableRow>
      <TableCell colSpan={999} sx={{ padding: "0 !important" }}>
        <EmptyState {...props} />
      </TableCell>
    </TableRow>
  );
};
