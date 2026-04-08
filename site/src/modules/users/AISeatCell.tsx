import { CircleCheck, X } from "lucide-react";
import type { FC } from "react";
import { TableCell } from "#/components/Table/Table";

interface AISeatCellProps {
	hasAISeat: boolean;
}

export const AISeatCell: FC<AISeatCellProps> = ({ hasAISeat }) => {
	return (
		<TableCell>
			{hasAISeat ? (
				<CircleCheck
					className="size-5 text-content-success"
					aria-label="Consuming AI seat"
				/>
			) : (
				<X
					className="size-5 text-content-disabled"
					aria-label="Not consuming AI seat"
				/>
			)}
		</TableCell>
	);
};
