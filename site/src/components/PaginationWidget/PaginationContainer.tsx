import type { PaginationResultInfo } from "hooks/usePaginatedQuery";
import type { FC, HTMLAttributes } from "react";
import { cn } from "utils/cn";
import { PaginationHeader } from "./PaginationHeader";
import { PaginationWidgetBase } from "./PaginationWidgetBase";

export type PaginationResult = PaginationResultInfo & {
	isPlaceholderData: boolean;
};

type PaginationProps = HTMLAttributes<HTMLDivElement> & {
	query: PaginationResult;
	paginationUnitLabel: string;
};

export const PaginationContainer: FC<PaginationProps> = ({
	children,
	query,
	paginationUnitLabel,
	...delegatedProps
}) => {
	return (
		<>
			<PaginationHeader
				limit={query.limit}
				totalRecords={query.totalRecords}
				currentOffsetStart={query.currentOffsetStart}
				paginationUnitLabel={paginationUnitLabel}
			/>

			<div
				{...delegatedProps}
				className={cn(
					"flex flex-col flex-nowrap row-gap-4",
					delegatedProps.className,
				)}
			>
				{children}

				{query.isSuccess && (
					<PaginationWidgetBase
						totalRecords={query.totalRecords}
						currentPage={query.currentPage}
						pageSize={query.limit}
						onPageChange={query.onPageChange}
						hasPreviousPage={query.hasPreviousPage}
						hasNextPage={query.hasNextPage}
					/>
				)}
			</div>
		</>
	);
};
