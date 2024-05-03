import type { UseQueryOptions, QueryKey } from "react-query";
import type { MetadataState, MetadataValue } from "hooks/useEmbeddedMetadata";

const disabledFetchOptions = {
  cacheTime: Infinity,
  staleTime: Infinity,
  refetchOnMount: false,
  refetchOnReconnect: false,
  refetchOnWindowFocus: false,
} as const satisfies UseQueryOptions;

type UseQueryOptionsWithMetadata<
  TMetadata extends MetadataValue = MetadataValue,
  TQueryFnData = unknown,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
> = Omit<
  UseQueryOptions<TQueryFnData, TError, TData, TQueryKey>,
  "initialData"
> & {
  metadata: MetadataState<TMetadata>;
};

type FormattedQueryOptionsResult<
  TQueryFnData = unknown,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
> = Omit<
  UseQueryOptions<TQueryFnData, TError, TData, TQueryKey>,
  "initialData"
> & {
  queryKey: NonNullable<TQueryKey>;
};

/**
 * cachedQuery allows the caller to only make a request a single time, and use
 * `initialData` if it is provided. This is particularly helpful for passing
 * values injected via metadata. We do this for the initial user fetch,
 * buildinfo, and a few others to reduce page load time.
 */
export function cachedQuery<
  TMetadata extends MetadataValue = MetadataValue,
  TQueryFnData = unknown,
  TError = unknown,
  TData = TQueryFnData,
  TQueryKey extends QueryKey = QueryKey,
>(
  options: UseQueryOptionsWithMetadata<
    TMetadata,
    TQueryFnData,
    TError,
    TData,
    TQueryKey
  >,
): FormattedQueryOptionsResult<TQueryFnData, TError, TData, TQueryKey> {
  const { metadata, ...delegatedOptions } = options;
  const newOptions = {
    ...delegatedOptions,
    initialData: metadata.available ? metadata.value : undefined,

    // Make sure the disabled options are always serialized last, so that no
    // one using this function can accidentally override the values
    ...(metadata.available ? disabledFetchOptions : {}),
  };

  return newOptions as FormattedQueryOptionsResult<
    TQueryFnData,
    TError,
    TData,
    TQueryKey
  >;
}
