import type { FC } from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";
import { cn } from "#/utils/cn";

type PaginationHeaderProps = {
	paginationUnitLabel: string;
	limit: number;
	totalRecords: number | undefined;
	currentOffsetStart: number | undefined;
	countIsCapped?: boolean;

	// Temporary escape hatch until Workspaces can be switched over to using
	// PaginationContainer
	className?: string;
};

export const PaginationAmount: FC<PaginationHeaderProps> = ({
	paginationUnitLabel,
	limit,
	totalRecords,
	currentOffsetStart,
	countIsCapped,
	className,
}) => {
	return (
		<div
			className={cn(
				"flex flex-row flex-nowrap items-center m-0",
				"text-[13px] text-content-secondary",
				"h-9", // The size of a small button
				"[&_strong]:text-content-primary",
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
							Showing <strong>{currentOffsetStart.toLocaleString()}</strong> to{" "}
							<strong>
								{(
									currentOffsetStart +
									(countIsCapped
										? limit - 1
										: Math.min(limit - 1, totalRecords - currentOffsetStart))
								).toLocaleString()}
							</strong>{" "}
							of{" "}
							<strong>
								{totalRecords.toLocaleString()}
								{countIsCapped && "+"}
							</strong>{" "}
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
