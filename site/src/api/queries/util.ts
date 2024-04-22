// cachedQuery allows the caller to only make a request
// a single time, and use `initialData` if it is provided.
//
// This is particularly helpful for passing values injected
// via metadata. We do this for the initial user fetch, buildinfo,
// and a few others to reduce page load time.
export const cachedQuery = {
  cacheTime: Infinity,
  staleTime: Infinity,
  refetchOnMount: false,
  refetchOnReconnect: false,
  refetchOnWindowFocus: false,
};
