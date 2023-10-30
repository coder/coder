import { type UseQueryOptions } from "react-query";
import { type BuildInfoResponse } from "api/typesGenerated";
import * as API from "api/api";
import { getMetadataAsJSON } from "utils/metadata";

const initialBuildInfoData = getMetadataAsJSON<BuildInfoResponse>("build-info");
const buildInfoKey = ["buildInfo"] as const;

export const buildInfo = (): UseQueryOptions<BuildInfoResponse> => {
  return {
    queryKey: buildInfoKey,
    initialData: initialBuildInfoData,
    queryFn: () => API.getBuildInfo(),
  };
};
