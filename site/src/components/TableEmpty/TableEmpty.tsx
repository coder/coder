import {
	EmptyState,
	type EmptyStateProps,
} from "components/EmptyState/EmptyState";
import { TableCell, TableRow } from "components/Table/Table";
import type { FC } from "react";

type TableEmptyProps = EmptyStateProps;

export const TableEmpty: FC<TableEmptyProps> = (props) => {
	return (
		<TableRow>
			<TableCell colSpan={999} className="p-0!">
				<EmptyState {...props} />
			</TableCell>
		</TableRow>
	);
};
