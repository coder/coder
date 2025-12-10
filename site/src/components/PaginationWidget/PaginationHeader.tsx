import Skeleton from "@mui/material/Skeleton";
import type { FC } from "react";
import { cn } from "utils/cn";

type PaginationHeaderProps = {
	paginationUnitLabel: string;
	limit: number;
	totalRecords: number | undefined;
	currentOffsetStart: number | undefined;

	// Temporary escape hatch until Workspaces can be switched over to using
	// PaginationContainer
	className?: string;
};

export const PaginationHeader: FC<PaginationHeaderProps> = ({
	paginationUnitLabel,
	limit,
	totalRecords,
	currentOffsetStart,
	className,
}) => {
	return (
		<div
			className={cn(
				"flex flex-nowrap items-center m-0 text-[13px] pb-2",
				"text-content-secondary [&_strong]:text-content-primary",
				"h-9", // The size of a small button
				className,
			)}
		>
			{totalRecords !== undefined ? (
				<>
					{/**
					 * Have to put text content in divs so that flexbox doesn't scramble
					 * the inner text nodes up
					 */}
					{totalRecords === 0 && <div>No records available</div>}

					{totalRecords !== 0 && currentOffsetStart !== undefined && (
						<div>
							Showing <strong>{currentOffsetStart}</strong> to{" "}
							<strong>
								{currentOffsetStart +
									Math.min(limit - 1, totalRecords - currentOffsetStart)}
							</strong>{" "}
							of <strong>{totalRecords.toLocaleString()}</strong>{" "}
							{paginationUnitLabel}
						</div>
					)}
				</>
			) : (
				<Skeleton variant="text" width={160} height={16} />
			)}
		</div>
	);
};
