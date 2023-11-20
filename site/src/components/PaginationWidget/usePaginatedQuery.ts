import { useEffect } from "react";
import { useEffectEvent } from "hooks/hookPolyfills";
import { useSearchParams } from "react-router-dom";

import { DEFAULT_RECORDS_PER_PAGE } from "./utils";
import { clamp } from "lodash";

import {
  type QueryFunctionContext,
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
 * A more specialized version of UseQueryOptions built specifically for
 * paginated queries.
 */
// All the type parameters just mirror the ones used by React Query
export type UsePaginatedQueryOptions<
  TQueryFnData extends PaginatedData = PaginatedData,
  TQueryPayload = never,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
> = BasePaginationOptions<TQueryFnData, TError, TData, TQueryKey> &
  QueryPayloadExtender<TQueryPayload> & {
    /**
     * A function that takes pagination information and produces a full query
     * key.
     *
     * Must be a function so that it can be used for the active query, and then
     * reused for any prefetching queries.
     */
    queryKey: (params: QueryPageParamsWithPayload<TQueryPayload>) => TQueryKey;

    /**
     * A version of queryFn that is required and that exposes the pagination
     * information through the pageParams context property
     */
    queryFn: (
      context: PaginatedQueryFnContext<TQueryKey, TQueryPayload>,
    ) => TQueryFnData | Promise<TQueryFnData>;

    /**
     * A custom, optional function for handling what happens if the user
     * navigates to a page that doesn't exist for the paginated data.
     *
     * If this function is not defined/provided, usePaginatedQuery will navigate
     * the user to the closest valid page.
     */
    onInvalidPage?: (currentPage: number, totalPages: number) => void;
  };

export function usePaginatedQuery<
  TQueryFnData extends PaginatedData = PaginatedData,
  TError = unknown,
  TData extends PaginatedData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
  TPayload = never,
>(
  options: UsePaginatedQueryOptions<
    TQueryFnData,
    TPayload,
    TError,
    TData,
    TQueryKey
  >,
) {
  const {
    queryKey,
    queryPayload,
    onInvalidPage,
    queryFn: outerQueryFn,
    ...extraOptions
  } = options;

  const [searchParams, setSearchParams] = useSearchParams();
  const currentPage = parsePage(searchParams);
  const pageSize = DEFAULT_RECORDS_PER_PAGE;
  const pageOffset = (currentPage - 1) * pageSize;

  const getQueryOptionsFromPage = (pageNumber: number) => {
    const pageParam: QueryPageParams = {
      pageNumber,
      pageOffset,
      pageSize,
      searchParams,
    };

    // eslint-disable-next-line @typescript-eslint/no-explicit-any -- Have to do this because proving the type's soundness to the compiler will make the code so much more convoluted and harder to maintain
    const payload = queryPayload?.(pageParam) as any;

    return {
      queryKey: queryKey({ ...pageParam, payload }),
      queryFn: (context: QueryFunctionContext<TQueryKey>) => {
        return outerQueryFn({ ...context, ...pageParam, payload });
      },
    } as const;
  };

  // Not using infinite query right now because that requires a fair bit of list
  // virtualization as the lists get bigger (especially for the audit logs).
  // Keeping initial implementation simple.
  const query = useQuery<TQueryFnData, TError, TData, TQueryKey>({
    ...extraOptions,
    ...getQueryOptionsFromPage(currentPage),
    keepPreviousData: true,
  });

  const totalRecords = query.data?.count ?? 0;
  const totalPages = Math.ceil(totalRecords / pageSize);
  const hasNextPage = pageSize * pageOffset < totalRecords;
  const hasPreviousPage = currentPage > 1;

  const queryClient = useQueryClient();
  const prefetchPage = useEffectEvent((newPage: number) => {
    return queryClient.prefetchQuery(getQueryOptionsFromPage(newPage));
  });

  // Have to split hairs and sync on both the current page and the hasXPage
  // variables, because the page can change immediately client-side, but the
  // hasXPage values are derived from the server and won't be immediately ready
  // on the initial render
  useEffect(() => {
    if (hasNextPage) {
      void prefetchPage(currentPage + 1);
    }
  }, [prefetchPage, currentPage, hasNextPage]);

  useEffect(() => {
    if (hasPreviousPage) {
      void prefetchPage(currentPage - 1);
    }
  }, [prefetchPage, currentPage, hasPreviousPage]);

  // Mainly here to catch user if they navigate to a page directly via URL
  const updatePageIfInvalid = useEffectEvent(() => {
    const clamped = clamp(currentPage, 1, totalPages);
    if (currentPage === clamped) {
      return;
    }

    if (onInvalidPage !== undefined) {
      onInvalidPage(currentPage, totalPages);
    } else {
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

    // Hijacking the isLoading property slightly because keepPreviousData is
    // true; by default, isLoading will always be false after the initial page
    // loads, even if new pages are loading in. Especially since
    // keepPreviousData is an implementation detail, simplifying the API felt
    // like the better option, at the risk of it becoming more "magical"
    isLoading: query.isLoading || query.isFetching,
  } as const;
}

function parsePage(params: URLSearchParams): number {
  const parsed = Number(params.get("page"));
  return Number.isInteger(parsed) && parsed > 1 ? parsed : 1;
}

/**
 * Papers over how the queryPayload function is defined at the type level, so
 * that UsePaginatedQueryOptions doesn't look as scary.
 *
 * You're going to see these tuple types in a few different spots in this file;
 * it's a "hack" to get around the function contravariance that pops up when you
 * normally try to share the TQueryPayload between queryPayload, queryKey, and
 * queryFn via the direct/"obvious" way. By throwing the types into tuples
 * (which are naturally covariant), it's a lot easier to share the types without
 * TypeScript complaining all the time or getting so confused that it degrades
 * the type definitions into a bunch of "any" types
 */
type QueryPayloadExtender<TQueryPayload = never> = [TQueryPayload] extends [
  never,
]
  ? { queryPayload?: never }
  : { queryPayload: (params: QueryPageParams) => TQueryPayload };

/**
 * Information about a paginated request. This information is passed into the
 * queryPayload, queryKey, and queryFn properties of the hook.
 */
type QueryPageParams = {
  pageNumber: number;
  pageSize: number;
  pageOffset: number;
  searchParams: URLSearchParams;
};

/**
 * The query page params, appended with the result of the queryPayload function.
 * This type is passed to both queryKey and queryFn. If queryPayload is
 * undefined, payload will always be undefined
 */
type QueryPageParamsWithPayload<TPayload = never> = QueryPageParams & {
  payload: [TPayload] extends [never] ? undefined : TPayload;
};

/**
 * Any JSON-serializable object returned by the API that exposes the total
 * number of records that match a query
 */
type PaginatedData = {
  count: number;
};

/**
 * React Query's QueryFunctionContext (minus pageParam, which is weird and
 * defaults to type any anyway), plus all properties from
 * QueryPageParamsWithPayload.
 */
type PaginatedQueryFnContext<
  TQueryKey extends QueryKey = QueryKey,
  TPayload = never,
> = Omit<QueryFunctionContext<TQueryKey>, "pageParam"> &
  QueryPageParamsWithPayload<TPayload>;

/**
 * The set of React Query properties that UsePaginatedQueryOptions derives from.
 *
 * Three properties are stripped from it:
 * - keepPreviousData - The value must always be true to keep pagination feeling
 *   nice, so better to prevent someone from trying to touch it at all
 * - queryFn - Removed to simplify replacing the type of its context argument
 * - queryKey - Removed so that it can be replaced with the function form of
 *   queryKey
 */
type BasePaginationOptions<
  TQueryFnData extends PaginatedData = PaginatedData,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
> = Omit<
  UseQueryOptions<TQueryFnData, TError, TData, TQueryKey>,
  "keepPreviousData" | "queryKey" | "queryFn"
>;
