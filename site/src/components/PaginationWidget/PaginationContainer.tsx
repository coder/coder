import { type FC, type HTMLAttributes } from "react";
import { type PaginationResultInfo } from "hooks/usePaginatedQuery";
import { PaginationWidgetBase } from "./PaginationWidgetBase";
import { PaginationHeader } from "./PaginationHeader";

export type PaginationResult = PaginationResultInfo & {
  isPreviousData: boolean;
};

type PaginationProps = HTMLAttributes<HTMLDivElement> & {
  paginationResult: PaginationResult;
  paginationUnitLabel: string;
};

export const PaginationContainer: FC<PaginationProps> = ({
  children,
  paginationResult,
  paginationUnitLabel,
  ...delegatedProps
}) => {
  return (
    <>
      <PaginationHeader
        limit={paginationResult.limit}
        totalRecords={paginationResult.totalRecords}
        currentOffsetStart={paginationResult.currentOffsetStart}
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

        {paginationResult.isSuccess && (
          <PaginationWidgetBase
            totalRecords={paginationResult.totalRecords}
            currentPage={paginationResult.currentPage}
            pageSize={paginationResult.limit}
            onPageChange={paginationResult.onPageChange}
            hasPreviousPage={paginationResult.hasPreviousPage}
            hasNextPage={paginationResult.hasNextPage}
          />
        )}
      </div>
    </>
  );
};
