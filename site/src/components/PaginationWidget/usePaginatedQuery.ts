import { useEffect } from "react";
import { useSearchParams } from "react-router-dom";
import { useEffectEvent } from "hooks/hookPolyfills";

import { type Pagination } from "api/typesGenerated";
import { DEFAULT_RECORDS_PER_PAGE } from "./utils";
import { prepareQuery } from "utils/filters";
import { clamp } from "lodash";

import {
  type QueryFunction,
  type QueryKey,
  type UseQueryOptions,
  useQueryClient,
  useQuery,
} from "react-query";

const PAGE_PARAMS_KEY = "page";
const PAGE_FILTER_KEY = "filter";

// Only omitting after_id for simplifying initial implementation; property
// should probably be added back in down the line
type PaginationInput = Omit<Pagination, "after_id"> & {
  q: string;
  limit: number;
  offset: number;
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
  prefetch?: boolean;

  /**
   * A function that takes pagination information and produces a full query key.
   *
   * Must be a function so that it can be used for the active query, as well as
   * any prefetching.
   */
  queryKey: (pagination: PaginationInput) => TQueryKey;

  /**
   * A version of queryFn that is required and that has access to the current
   * page via the pageParams context property
   */
  queryFn: QueryFunction<TQueryFnData, TQueryKey, number>;
};

export function usePaginatedQuery<
  TQueryFnData extends PaginatedData = PaginatedData,
  TError = unknown,
  TData extends PaginatedData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
>(options: UsePaginatedQueryOptions<TQueryFnData, TError, TData, TQueryKey>) {
  const { queryKey, queryFn, prefetch = false, ...otherOptions } = options;

  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parsePage(searchParams);

  const pageSize = DEFAULT_RECORDS_PER_PAGE;
  const pageOffset = (currentPage - 1) * pageSize;

  const query = useQuery({
    ...otherOptions,
    queryFn: (queryCxt) => queryFn({ ...queryCxt, pageParam: currentPage }),
    queryKey: queryKey({
      q: preparePageQuery(searchParams, currentPage),
      limit: pageSize,
      offset: pageOffset,
    }),
    keepPreviousData: true,
  });

  const queryClient = useQueryClient();
  const prefetchPage = useEffectEvent((newPage: number) => {
    if (!prefetch) {
      return;
    }

    void queryClient.prefetchQuery({
      queryFn: (queryCxt) => queryFn({ ...queryCxt, pageParam: newPage }),
      queryKey: queryKey({
        q: preparePageQuery(searchParams, newPage),
        limit: pageSize,
        offset: pageOffset,
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
      searchParams.set(PAGE_PARAMS_KEY, String(clamped));
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

    searchParams.set(PAGE_PARAMS_KEY, String(safePage));
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
    isLoading: query.isLoading || query.isFetching,
  } as const;
}

function parsePage(params: URLSearchParams): number {
  const parsed = Number(params.get("page"));
  return Number.isInteger(parsed) && parsed > 1 ? parsed : 1;
}

function preparePageQuery(searchParams: URLSearchParams, page: number) {
  const paramsPage = Number(searchParams.get(PAGE_FILTER_KEY));

  let queryText: string;
  if (paramsPage === page) {
    queryText = searchParams.toString();
  } else {
    const newParams = new URLSearchParams(searchParams);
    newParams.set(PAGE_FILTER_KEY, String(page));
    queryText = newParams.toString();
  }

  return prepareQuery(queryText);
}
