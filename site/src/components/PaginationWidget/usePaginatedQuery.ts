import { useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { useEffectEvent } from "hooks/hookPolyfills";
import { DEFAULT_RECORDS_PER_PAGE } from "./utils";

import {
  type QueryKey,
  type UseQueryOptions,
  useQueryClient,
  useQuery,
} from "react-query";

const PAGE_PARAMS_KEY = "page";

// Any JSON-serializable object with a count property representing the total
// number of records for a given resource
type PaginatedData = {
  count: number;
};

// All the type parameters just mirror the ones used by React Query
type PaginatedOptions<
  TQueryFnData extends PaginatedData = PaginatedData,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
> = Omit<
  UseQueryOptions<TQueryFnData, TError, TData, TQueryKey>,
  "keepPreviousData" | "queryKey"
> & {
  /**
   * A function that takes a page number and produces a full query key. Must be
   * a function so that it can be used for the active query, as well as
   * prefetching
   */
  queryKey: (pageNumber: number) => TQueryKey;

  searchParamsResult: ReturnType<typeof useSearchParams>;
  prefetchNextPage?: boolean;
};

export function usePaginatedQuery<
  TQueryFnData extends PaginatedData = PaginatedData,
  TError = unknown,
  TData extends PaginatedData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
>(options: PaginatedOptions<TQueryFnData, TError, TData, TQueryKey>) {
  const { searchParamsResult, queryKey, prefetchNextPage = false } = options;
  const [searchParams, setSearchParams] = searchParamsResult;
  const currentPage = parsePage(searchParams);

  // Can't use useInfiniteQuery because that hook is designed to work with data
  // that streams in and can't be cut up into pages easily
  const query = useQuery({
    ...options,
    queryKey: queryKey(currentPage),
    keepPreviousData: true,
  });

  const pageSize = DEFAULT_RECORDS_PER_PAGE;
  const pageOffset = (currentPage - 1) * pageSize;
  const totalRecords = query.data?.count ?? 0;
  const totalPages = Math.ceil(totalRecords / pageSize);
  const hasPreviousPage = currentPage > 1;
  const hasNextPage = pageSize * pageOffset < totalRecords;

  const queryClient = useQueryClient();
  const prefetch = useEffectEvent((newPage: number) => {
    if (!prefetchNextPage) {
      return;
    }

    const newKey = queryKey(newPage);
    void queryClient.prefetchQuery(newKey);
  });

  useEffect(() => {
    if (hasPreviousPage) {
      prefetch(currentPage - 1);
    }

    if (hasNextPage) {
      prefetch(currentPage + 1);
    }
  }, [prefetch, currentPage, hasNextPage, hasPreviousPage]);

  // Tries to redirect a user if they navigate to a page via invalid URL
  const navigateIfInvalidPage = useEffectEvent(
    (currentPage: number, totalPages: number) => {
      const clamped = Math.max(1, Math.min(currentPage, totalPages));

      if (currentPage !== clamped) {
        searchParams.set(PAGE_PARAMS_KEY, String(clamped));
        setSearchParams(searchParams);
      }
    },
  );

  useEffect(() => {
    navigateIfInvalidPage(currentPage, totalPages);
  }, [navigateIfInvalidPage, currentPage, totalPages]);

  const onPageChange = (newPage: number) => {
    const safePage = Number.isInteger(newPage)
      ? Math.max(1, Math.min(newPage))
      : 1;

    searchParams.set(PAGE_PARAMS_KEY, String(safePage));
    setSearchParams(searchParams);
  };

  return {
    ...query,
    onPageChange,
    currentPage,
    totalRecords,
    hasNextPage,
    pageSize,
    isLoading: query.isLoading || query.isFetching,
  } as const;
}

function parsePage(params: URLSearchParams): number {
  const parsed = Number(params.get("page"));
  return Number.isInteger(parsed) && parsed > 1 ? parsed : 1;
}
