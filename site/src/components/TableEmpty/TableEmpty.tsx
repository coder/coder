import TableCell from "@mui/material/TableCell";
import TableRow from "@mui/material/TableRow";
import {
	EmptyState,
	type EmptyStateProps,
} from "components/EmptyState/EmptyState";
import type { FC } from "react";

export type TableEmptyProps = EmptyStateProps;

export const TableEmpty: FC<TableEmptyProps> = (props) => {
	return (
		<TableRow>
			<TableCell colSpan={999} css={{ padding: "0 !important" }}>
				<EmptyState {...props} />
			</TableCell>
		</TableRow>
	);
};
