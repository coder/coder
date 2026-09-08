import { cn } from "cn";
import type { FC } from "react";
import { Skeleton } from "#/components/Skeleton/Skeleton";

type PaginationHeaderProps = {
	paginationUnitLabel: string;
	limit: number;
	totalRecords: number | undefined;
	currentOffsetStart: number | undefined;
	countIsCapped?: boolean;
	/** Prefixes the summary with "Filtered:" when a filter narrows the results. */
	isFiltered?: boolean;

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
	isFiltered = false,
	className,
}) => {
	return (
		<div
			className={cn(
				"flex flex-row flex-nowrap items-center m-0",
				"text-xs font-normal text-content-secondary",
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
					{totalRecords === 0 && (
						<div>
							{isFiltered
								? `No ${paginationUnitLabel} match your search.`
								: "No records available"}
						</div>
					)}

					{totalRecords !== 0 && currentOffsetStart !== undefined && (
						<div>
							{isFiltered && "Filtered: "}
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
