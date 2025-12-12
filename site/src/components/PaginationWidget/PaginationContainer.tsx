import type { PaginationResultInfo } from "hooks/usePaginatedQuery";
import type { FC, HTMLAttributes } from "react";
import { PaginationAmount } from "./PaginationHeader";
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
		<div
			css={{
				display: "flex",
				flexFlow: "column nowrap",
				rowGap: "16px",
			}}
			{...delegatedProps}
		>
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
