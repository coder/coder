import { type FC, type HTMLAttributes } from "react";
import { type PaginationResultInfo } from "hooks/usePaginatedQuery";
import { PaginationWidgetBase } from "./PaginationWidgetBase";
import { PaginationHeader } from "./PaginationHeader";

export type PaginationResult = PaginationResultInfo & {
  isPreviousData: boolean;
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
    </>
  );
};
