import type { UseQueryOptions } from "react-query";
import * as API from "api/api";
import type { BuildInfoResponse } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";

const initialBuildInfoData = getMetadataAsJSON<BuildInfoResponse>("build-info");
const buildInfoKey = ["buildInfo"] as const;

export const buildInfo = () => {
  const opts: UseQueryOptions<BuildInfoResponse> = {
    queryKey: buildInfoKey,
    initialData: initialBuildInfoData,
    queryFn: () => API.getBuildInfo(),
  };
  // If we have initial build info data, we don't want to fetch
  // the user again. We already have it!
  if (initialBuildInfoData) {
    opts.cacheTime = Infinity;
    opts.staleTime = Infinity;
    opts.refetchOnMount = false;
    opts.refetchOnReconnect = false;
    opts.refetchOnWindowFocus = false;
  }
  return opts;
};
