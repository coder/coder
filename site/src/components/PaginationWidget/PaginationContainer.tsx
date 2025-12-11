import type { PaginationResultInfo } from "hooks/usePaginatedQuery";
import type { FC, HTMLAttributes } from "react";
import { PaginationHeader } from "./PaginationHeader";
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
		<>
			{paginationPosition === "top" && (
				<PaginationHeader
					limit={query.limit}
					totalRecords={query.totalRecords}
					currentOffsetStart={query.currentOffsetStart}
					paginationUnitLabel={paginationUnitLabel}
				/>
			)}

			<div
				css={{
					display: "flex",
					flexFlow: "column nowrap",
					rowGap: "16px",
				}}
				{...delegatedProps}
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

			{paginationPosition === "bottom" && (
				<PaginationHeader
					limit={query.limit}
					totalRecords={query.totalRecords}
					currentOffsetStart={query.currentOffsetStart}
					paginationUnitLabel={paginationUnitLabel}
					className="pt-8 justify-end"
				/>
			)}
		</>
	);
};
