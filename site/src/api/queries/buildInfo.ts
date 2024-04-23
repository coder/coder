import type { UseQueryOptions } from "react-query";
import * as API from "api/api";
import type { BuildInfoResponse } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";
import { cachedQuery } from "./util";

const initialBuildInfoData = getMetadataAsJSON<BuildInfoResponse>("build-info");
const buildInfoKey = ["buildInfo"] as const;

export const buildInfo = (): UseQueryOptions<BuildInfoResponse> => {
  // The version of the app can't change without reloading the page.
  return cachedQuery({
    initialData: initialBuildInfoData,
    queryKey: buildInfoKey,
    queryFn: () => API.getBuildInfo(),
  });
};
