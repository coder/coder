import { TableCell, TableRow } from "components/Table/Table";
import { ChevronDownIcon, ChevronRightIcon } from "lucide-react";
import { type FC, useState } from "react";
import { cn } from "utils/cn";

// biome-ignore lint/complexity/noBannedTypes: TODO
type RequestLogsRowProps = {};

export const RequestLogsRow: FC<RequestLogsRowProps> = () => {
	const [isOpen, setIsOpen] = useState(false);

	return (
		<>
			<TableRow
				className={cn("select-none cursor-pointer hover:bg-surface-secondary")}
				onClick={() => setIsOpen(!isOpen)}
			>
				<TableCell>
					<div
						css={{
							display: "flex",
							alignItems: "center",
							justifyContent: "center",
						}}
					>
						{isOpen ? (
							<ChevronDownIcon size={16} />
						) : (
							<ChevronRightIcon size={16} />
						)}
					</div>
				</TableCell>
				<TableCell>2025-01-01 00:00:00</TableCell>
				<TableCell>John Doe</TableCell>
				<TableCell>This is a prompt</TableCell>
				<TableCell>100</TableCell>
				<TableCell>1</TableCell>
				<TableCell>Status</TableCell>
			</TableRow>
			{isOpen && (
				<TableRow>
					<TableCell colSpan={999}>TODO</TableCell>
				</TableRow>
			)}
		</>
	);
};
