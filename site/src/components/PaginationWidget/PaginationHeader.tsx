import { useTheme } from "@emotion/react";
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
	const theme = useTheme();

	return (
		<div
			css={{
				color: theme.palette.text.secondary,
				"& strong": {
					color: theme.palette.text.primary,
				},
			}}
			className={cn(
				className,
				"flex flex-nowrap items-center m-0 text-[13px] pb-2",
				"h-9", // The size of a small button
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
