import { CheckIcon } from "lucide-react";
import type { FC } from "react";
import { TableCell } from "#/components/Table/Table";

interface AISeatCellProps {
	hasAISeat: boolean;
}

export const AISeatCell: FC<AISeatCellProps> = ({ hasAISeat }) => {
	return (
		<TableCell>
			{hasAISeat ? (
				<CheckIcon
					className="size-5 text-content-success"
					aria-label="Consuming AI seat"
				/>
			) : (
				<span
					role="img"
					aria-label="Not consuming AI seat"
					className="text-content-disabled"
				>
					&mdash;
				</span>
			)}
		</TableCell>
	);
};
