import { useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { useEffectEvent } from "hooks/hookPolyfills";

import { DEFAULT_RECORDS_PER_PAGE } from "./utils";
import { clamp } from "lodash";

import {
  type QueryFunction,
  type QueryKey,
  type UseQueryOptions,
  useQueryClient,
  useQuery,
} from "react-query";

/**
 * The key to use for getting/setting the page number from the search params
 */
const PAGE_NUMBER_PARAMS_KEY = "page";

/**
 * All arguments passed into the queryKey functions.
 */
type QueryKeyFnArgs = {
  pageNumber: number;
  pageSize: number;
  pageOffset: number;
  extraQuery?: string;
};

/**
 * Any JSON-serializable object returned by the API that exposes the total
 * number of records that match a query
 */
interface PaginatedData {
  count: number;
}

/**
 * A more specialized version of UseQueryOptions built specifically for
 * paginated queries.
 */
// All the type parameters just mirror the ones used by React Query
export type UsePaginatedQueryOptions<
  TQueryFnData extends PaginatedData = PaginatedData,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
> = Omit<
  UseQueryOptions<TQueryFnData, TError, TData, TQueryKey>,
  "keepPreviousData" | "queryKey"
> & {
  /**
   * A function that takes pagination information and produces a full query key.
   *
   * Must be a function so that it can be used for the active query, as well as
   * any prefetching.
   */
  queryKey: (args: QueryKeyFnArgs) => TQueryKey;

  /**
   * A version of queryFn that is required and that exposes page numbers through
   * the pageParams context property
   */
  queryFn: QueryFunction<TQueryFnData, TQueryKey, number>;
};

export function usePaginatedQuery<
  TQueryFnData extends PaginatedData = PaginatedData,
  TError = unknown,
  TData extends PaginatedData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
>(options: UsePaginatedQueryOptions<TQueryFnData, TError, TData, TQueryKey>) {
  const { queryKey, queryFn, ...otherOptions } = options;
  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parsePage(searchParams);

  const pageSize = DEFAULT_RECORDS_PER_PAGE;
  const pageOffset = (currentPage - 1) * pageSize;

  // Not using infinite query right now because that requires a fair bit of list
  // virtualization as the lists get bigger (especially for the audit logs)
  const query = useQuery({
    ...otherOptions,
    queryFn: (queryCxt) => queryFn({ ...queryCxt, pageParam: currentPage }),
    queryKey: queryKey({
      pageNumber: currentPage,
      pageSize,
      pageOffset,
    }),
    keepPreviousData: true,
  });

  const queryClient = useQueryClient();
  const prefetchPage = useEffectEvent((newPage: number) => {
    void queryClient.prefetchQuery({
      queryFn: (queryCxt) => queryFn({ ...queryCxt, pageParam: newPage }),
      queryKey: queryKey({
        pageNumber: newPage,
        pageSize,
        pageOffset,
      }),
    });
  });

  const totalRecords = query.data?.count ?? 0;
  const totalPages = Math.ceil(totalRecords / pageSize);
  const hasNextPage = pageSize * pageOffset < totalRecords;
  const hasPreviousPage = currentPage > 1;

  // Have to split hairs and sync on both the current page and the hasXPage
  // variables because hasXPage values are derived from server values and aren't
  // immediately ready on each render
  useEffect(() => {
    if (hasNextPage) {
      prefetchPage(currentPage + 1);
    }
  }, [prefetchPage, currentPage, hasNextPage]);

  useEffect(() => {
    if (hasPreviousPage) {
      prefetchPage(currentPage - 1);
    }
  }, [prefetchPage, currentPage, hasPreviousPage]);

  // Mainly here to catch user if they navigate to a page directly via URL
  const updatePageIfInvalid = useEffectEvent(() => {
    const clamped = clamp(currentPage, 1, totalPages);

    if (currentPage !== clamped) {
      searchParams.set(PAGE_NUMBER_PARAMS_KEY, String(clamped));
      setSearchParams(searchParams);
    }
  });

  useEffect(() => {
    if (!query.isFetching) {
      updatePageIfInvalid();
    }
  }, [updatePageIfInvalid, query.isFetching]);

  const onPageChange = (newPage: number) => {
    const safePage = Number.isInteger(newPage)
      ? clamp(newPage, 1, totalPages)
      : 1;

    searchParams.set(PAGE_NUMBER_PARAMS_KEY, String(safePage));
    setSearchParams(searchParams);
  };

  return {
    ...query,
    onPageChange,
    goToPreviousPage: () => onPageChange(currentPage - 1),
    goToNextPage: () => onPageChange(currentPage + 1),
    currentPage,
    pageSize,
    totalRecords,
    hasNextPage,
    hasPreviousPage,

    // Have to hijack the isLoading property slightly because keepPreviousData
    // is true; by default, isLoading will be false after the initial page
    // loads, even if new pages are loading in
    isLoading: query.isLoading || query.isFetching,
  } as const;
}

function parsePage(params: URLSearchParams): number {
  const parsed = Number(params.get("page"));
  return Number.isInteger(parsed) && parsed > 1 ? parsed : 1;
}
