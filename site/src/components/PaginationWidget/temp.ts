export function usePaginatedQuery(options: any) {
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
    const pageParams = {
      pageNumber,
      offset: pageOffset,
      limit: pageSize,
      searchParams: searchParams,
    };

    const payload = queryPayload?.(pageParams);

    return {
      queryKey: queryKey({ ...pageParams, payload }),
      queryFn: (context) => {
        return outerQueryFn({ ...context, ...pageParams, payload });
      },
    } as const;
  };

  const query = useQuery({
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

    if (onInvalidPage === undefined) {
      searchParams.set(PAGE_NUMBER_PARAMS_KEY, String(clamped));
      setSearchParams(searchParams);
    } else {
      const params = {
        offset: pageOffset,
        limit: pageSize,
        totalPages,
        setSearchParams,
        pageNumber: currentPage,
        searchParams: searchParams,
      };

      onInvalidPage(params);
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
    isLoading: query.isLoading || query.isFetching,
  } as const;
}
