import { TableCell } from "components/Table/Table";
import { CircleCheck, X } from "lucide-react";
import type { FC } from "react";

interface AISeatCellProps {
	hasAISeat: boolean;
}

export const AISeatCell: FC<AISeatCellProps> = ({ hasAISeat }) => {
	return (
		<TableCell>
			{hasAISeat ? (
				<CircleCheck className="size-5 text-content-success" />
			) : (
				<X className="size-5 text-content-disabled" />
			)}
		</TableCell>
	);
};
