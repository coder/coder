import type { UseQueryOptions } from "react-query";
import * as API from "api/api";
import type { BuildInfoResponse } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";
import { cachedQuery } from "./util";

const initialBuildInfoData = getMetadataAsJSON<BuildInfoResponse>("build-info");
const buildInfoKey = ["buildInfo"] as const;

export const buildInfo = (): UseQueryOptions<BuildInfoResponse> => {
  return {
    // We either have our initial data or should immediately
    // fetch and never again!
    ...cachedQuery(initialBuildInfoData),
    queryKey: buildInfoKey,
    queryFn: () => API.getBuildInfo(),
  };
};
