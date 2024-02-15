import { useEffect } from "react";
import { useEffectEvent } from "./hookPolyfills";
import { type SetURLSearchParams, useSearchParams } from "react-router-dom";
import clamp from "lodash/clamp";

import {
  type QueryFunctionContext,
  type QueryKey,
  type UseQueryOptions,
  type UseQueryResult,
  useQueryClient,
  useQuery,
} from "react-query";

const DEFAULT_RECORDS_PER_PAGE = 25;

/**
 * The key to use for getting/setting the page number from the search params
 */
const PAGE_NUMBER_PARAMS_KEY = "page";

/**
 * A more specialized version of UseQueryOptions built specifically for
 * paginated queries.
 */
export type UsePaginatedQueryOptions<
  // Aside from TQueryPayload, all type parameters come from the base React
  // Query type definition, and are here for compatibility
  TQueryFnData extends PaginatedData = PaginatedData,
  TQueryPayload = never,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
> = BasePaginationOptions<TQueryFnData, TError, TData, TQueryKey> &
  QueryPayloadExtender<TQueryPayload> & {
    /**
     * An optional dependency for React Router's URLSearchParams. If this is
     * provided, all URL state changes will go through this object instead of
     * an internal value.
     */
    searchParams?: URLSearchParams;

    /**
     * A function that takes pagination information and produces a full query
     * key.
     *
     * Must be a function so that it can be used for the active query, and then
     * reused for any prefetching queries (swapping the page number out).
     */
    queryKey: (params: QueryPageParamsWithPayload<TQueryPayload>) => TQueryKey;

    /**
     * A version of queryFn that is required and that exposes the pagination
     * information through its query function context argument
     */
    queryFn: (
      context: PaginatedQueryFnContext<TQueryKey, TQueryPayload>,
    ) => TQueryFnData | Promise<TQueryFnData>;

    /**
     * A custom, optional function for handling what happens if the user
     * navigates to a page that doesn't exist for the paginated data.
     *
     * If this function is not defined/provided when an invalid page is
     * encountered, usePaginatedQuery will default to navigating the user to the
     * closest valid page.
     */
    onInvalidPageChange?: (params: InvalidPageParams) => void;

    /**
     * Defaults to true. Allows you to disable prefetches for pages where making
     * a request is very expensive.
     */
    prefetch?: boolean;
  };

/**
 * The result of calling usePaginatedQuery. Mirrors the result of the base
 * useQuery as closely as possible, while adding extra pagination properties
 */
export type UsePaginatedQueryResult<
  TData = unknown,
  TError = unknown,
> = UseQueryResult<TData, TError> & PaginationResultInfo;

export function usePaginatedQuery<
  TQueryFnData extends PaginatedData = PaginatedData,
  TQueryPayload = never,
  TError = unknown,
  TData extends PaginatedData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
>(
  options: UsePaginatedQueryOptions<
    TQueryFnData,
    TQueryPayload,
    TError,
    TData,
    TQueryKey
  >,
): UsePaginatedQueryResult<TData, TError> {
  const {
    queryKey,
    queryPayload,
    onInvalidPageChange,
    searchParams: outerSearchParams,
    queryFn: outerQueryFn,
    prefetch = true,
    staleTime = 60 * 1000, // One minute
    ...extraOptions
  } = options;

  const [innerSearchParams, setSearchParams] = useSearchParams();
  const searchParams = outerSearchParams ?? innerSearchParams;

  const limit = DEFAULT_RECORDS_PER_PAGE;
  const currentPage = parsePage(searchParams);
  const currentPageOffset = (currentPage - 1) * limit;

  type Options = UseQueryOptions<TQueryFnData, TError, TData, TQueryKey>;
  const getQueryOptionsFromPage = (pageNumber: number): Options => {
    const pageParams: QueryPageParams = {
      pageNumber,
      limit,
      offset: (pageNumber - 1) * limit,
      searchParams: getParamsWithoutPage(searchParams),
    };

    const payload = queryPayload?.(pageParams) as RuntimePayload<TQueryPayload>;
    return {
      staleTime,
      queryKey: queryKey({ ...pageParams, payload }),
      queryFn: (context: QueryFunctionContext<TQueryKey>) => {
        return outerQueryFn({ ...context, ...pageParams, payload });
      },
    };
  };

  // Not using infinite query right now because that requires a fair bit of list
  // virtualization as the lists get bigger (especially for the audit logs).
  // Keeping initial implementation simple.
  const query = useQuery<TQueryFnData, TError, TData, TQueryKey>({
    ...extraOptions,
    ...getQueryOptionsFromPage(currentPage),
    keepPreviousData: true,
  });

  const totalRecords = query.data?.count;
  const totalPages =
    totalRecords !== undefined ? Math.ceil(totalRecords / limit) : undefined;

  const hasNextPage =
    totalRecords !== undefined && limit + currentPageOffset < totalRecords;
  const hasPreviousPage =
    totalRecords !== undefined &&
    currentPage > 1 &&
    currentPageOffset - limit < totalRecords;

  const queryClient = useQueryClient();
  const prefetchPage = useEffectEvent((newPage: number) => {
    if (!prefetch) {
      return;
    }

    const options = getQueryOptionsFromPage(newPage);
    return queryClient.prefetchQuery(options);
  });

  // Have to split hairs and sync on both the current page and the hasXPage
  // variables, because the page can change immediately client-side, but the
  // hasXPage values are derived from the server and won't always be immediately
  // ready on the initial render
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

  // Mainly here to catch user if they navigate to a page directly via URL;
  // totalPages parameterized to insulate function from fetch status changes
  const updatePageIfInvalid = useEffectEvent(async (totalPages: number) => {
    // If totalPages is 0, that's a sign that the currentPage overshot, and the
    // API returned a count of 0 because it didn't know how to process the query
    let fixedTotalPages: number;
    if (totalPages !== 0) {
      fixedTotalPages = totalPages;
    } else {
      const firstPageOptions = getQueryOptionsFromPage(1);
      try {
        const firstPageResult = await queryClient.fetchQuery(firstPageOptions);
        const rounded = Math.ceil(firstPageResult?.count ?? 0 / limit);
        fixedTotalPages = Math.max(rounded, 1);
      } catch {
        fixedTotalPages = 1;
      }
    }

    const clamped = clamp(currentPage, 1, fixedTotalPages);
    if (currentPage === clamped) {
      return;
    }

    const withoutPage = getParamsWithoutPage(searchParams);
    if (onInvalidPageChange === undefined) {
      withoutPage.set(PAGE_NUMBER_PARAMS_KEY, String(clamped));
      setSearchParams(withoutPage);
    } else {
      const params: InvalidPageParams = {
        limit,
        setSearchParams,
        offset: currentPageOffset,
        searchParams: withoutPage,
        totalPages: fixedTotalPages,
        pageNumber: currentPage,
      };

      onInvalidPageChange(params);
    }
  });

  useEffect(() => {
    if (!query.isFetching && totalPages !== undefined) {
      void updatePageIfInvalid(totalPages);
    }
  }, [updatePageIfInvalid, query.isFetching, totalPages]);

  const onPageChange = (newPage: number) => {
    // Page 1 is the only page that can be safely navigated to without knowing
    // totalPages; no reliance on server data for math calculations
    if (totalPages === undefined && newPage !== 1) {
      return;
    }

    const cleanedInput = clamp(Math.trunc(newPage), 1, totalPages ?? 1);
    if (Number.isNaN(cleanedInput)) {
      return;
    }

    searchParams.set(PAGE_NUMBER_PARAMS_KEY, String(cleanedInput));
    setSearchParams(searchParams);
  };

  // Have to do a type assertion for final return type to make React Query's
  // internal types happy; splitting type definitions up to limit risk of the
  // type assertion silencing type warnings we actually want to pay attention to
  const info: PaginationResultInfo = {
    limit,
    currentPage,
    onPageChange,
    goToFirstPage: () => onPageChange(1),

    goToPreviousPage: () => {
      if (hasPreviousPage) {
        onPageChange(currentPage - 1);
      }
    },

    goToNextPage: () => {
      if (hasNextPage) {
        onPageChange(currentPage + 1);
      }
    },

    ...(query.isSuccess
      ? {
          isSuccess: true,
          hasNextPage,
          hasPreviousPage,
          totalRecords: totalRecords as number,
          totalPages: totalPages as number,
          currentOffsetStart: currentPageOffset + 1,
        }
      : {
          isSuccess: false,
          hasNextPage: false,
          hasPreviousPage: false,
          totalRecords: undefined,
          totalPages: undefined,
          currentOffsetStart: undefined,
        }),
  };

  return { ...query, ...info } as UsePaginatedQueryResult<TData, TError>;
}

function parsePage(params: URLSearchParams): number {
  const parsed = Number(params.get("page"));
  return Number.isInteger(parsed) && parsed > 1 ? parsed : 1;
}

/**
 * Strips out the page number from a query so that there aren't mismatches
 * between it and usePaginatedQuery's currentPage property (especially for
 * prefetching)
 */
function getParamsWithoutPage(params: URLSearchParams): URLSearchParams {
  const withoutPage = new URLSearchParams(params);
  withoutPage.delete(PAGE_NUMBER_PARAMS_KEY);
  return withoutPage;
}

/**
 * All the pagination-properties for UsePaginatedQueryResult. Split up so that
 * the types can be used separately in multiple spots.
 */
export type PaginationResultInfo = {
  currentPage: number;
  limit: number;
  onPageChange: (newPage: number) => void;
  goToPreviousPage: () => void;
  goToNextPage: () => void;
  goToFirstPage: () => void;
} & (
  | {
      isSuccess: false;
      hasNextPage: false;
      hasPreviousPage: false;
      totalRecords: undefined;
      totalPages: undefined;
      currentOffsetStart: undefined;
    }
  | {
      isSuccess: true;
      hasNextPage: boolean;
      hasPreviousPage: boolean;
      totalRecords: number;
      totalPages: number;
      currentOffsetStart: number;
    }
);

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
  : {
      /**
       * An optional function for defining reusable "patterns" for taking
       * pagination data (current page, etc.), which will be evaluated and
       * passed to queryKey and queryFn for active queries and prefetch queries.
       *
       * queryKey and queryFn can each access the result of queryPayload
       * by accessing the "payload" property from their main function argument
       */
      queryPayload: (params: QueryPageParams) => TQueryPayload;
    };

/**
 * Information about a paginated request. This information is passed into the
 * queryPayload, queryKey, and queryFn properties of the hook.
 */
type QueryPageParams = {
  /**
   * The page number used when evaluating queryKey and queryFn. pageNumber will
   * be the current page during rendering, but will be the next/previous pages
   * for any prefetching.
   */
  pageNumber: number;

  /**
   * The number of data records to pull per query. Currently hard-coded based
   * off the value from PaginationWidget's utils file
   */
  limit: number;

  /**
   * The page offset to use for querying. Just here for convenience; can also be
   * derived from pageNumber and limit
   */
  offset: number;

  /**
   * The current URL search params. Useful for letting you grab certain search
   * terms from the URL
   */
  searchParams: URLSearchParams;
};

/**
 * Weird, hard-to-describe type definition, but it's necessary for making sure
 * that the type information involving the queryPayload function narrows
 * properly.
 */
type RuntimePayload<TPayload = never> = [TPayload] extends [never]
  ? undefined
  : TPayload;

/**
 * The query page params, appended with the result of the queryPayload function.
 * This type is passed to both queryKey and queryFn. If queryPayload is
 * undefined, payload will always be undefined
 */
type QueryPageParamsWithPayload<TPayload = never> = QueryPageParams & {
  payload: RuntimePayload<TPayload>;
};

/**
 * Any JSON-serializable object returned by the API that exposes the total
 * number of records that match a query
 */
export type PaginatedData = {
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
 * - queryFn - Removed to make it easier to swap in a custom queryFn type
 *   definition with a custom context argument
 * - queryKey - Removed so that it can be replaced with the function form of
 *   queryKey
 * - onSuccess/onError - APIs are deprecated and removed in React Query v5
 */
type BasePaginationOptions<
  TQueryFnData extends PaginatedData = PaginatedData,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
> = Omit<
  UseQueryOptions<TQueryFnData, TError, TData, TQueryKey>,
  "keepPreviousData" | "queryKey" | "queryFn" | "onSuccess" | "onError"
>;

/**
 * The argument passed to a custom onInvalidPageChange callback.
 */
type InvalidPageParams = QueryPageParams & {
  totalPages: number;
  setSearchParams: SetURLSearchParams;
};
