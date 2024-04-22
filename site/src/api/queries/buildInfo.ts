import type { UseQueryOptions } from "react-query";
import * as API from "api/api";
import type { BuildInfoResponse } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";

const initialBuildInfoData = getMetadataAsJSON<BuildInfoResponse>("build-info");
const buildInfoKey = ["buildInfo"] as const;

export const buildInfo = (): UseQueryOptions<BuildInfoResponse> => {
  return {
    // We either have our initial data or should immediately
    // fetch and never again!
    cacheTime: Infinity,
    staleTime: Infinity,
    refetchOnMount: false,
    refetchOnReconnect: false,
    refetchOnWindowFocus: false,
    queryKey: buildInfoKey,
    initialData: initialBuildInfoData,
    queryFn: () => API.getBuildInfo(),
  };
};
