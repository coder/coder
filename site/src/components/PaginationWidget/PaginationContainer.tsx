import type { PaginationResultInfo } from "hooks/usePaginatedQuery";
import type { FC, HTMLAttributes } from "react";
import { PaginationAmount } from "./PaginationAmount";
import { PaginationWidgetBase } from "./PaginationWidgetBase";

export type PaginationResult = PaginationResultInfo & {
	isPlaceholderData: boolean;
};

type PaginationProps = HTMLAttributes<HTMLDivElement> & {
	query: PaginationResult;
	paginationUnitLabel: string;
	paginationPosition?: "top" | "bottom";
};

export const PaginationContainer: FC<PaginationProps> = ({
	children,
	query,
	paginationUnitLabel,
	paginationPosition = "top",
	...delegatedProps
}) => {
	return (
		<div className="flex flex-col gap-y-4" {...delegatedProps}>
			{children}

			<PaginationAmount
				limit={query.limit}
				totalRecords={query.totalRecords}
				currentOffsetStart={query.currentOffsetStart}
				paginationUnitLabel={paginationUnitLabel}
				className="justify-end"
			/>

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
	);
};
