import { QueryClient, type UseQueryOptions } from "react-query";
import { type BuildInfoResponse } from "api/typesGenerated";
import * as API from "api/api";
import { getMetadataAsJSON } from "utils/metadata";

const initialBuildInfoData = getMetadataAsJSON<BuildInfoResponse>("build-info");
const buildInfoKey = ["buildInfo"] as const;

export const buildInfo = (queryClient: QueryClient) => {
  return {
    queryKey: buildInfoKey,
    queryFn: async () => {
      const cachedData = queryClient.getQueryData(buildInfoKey);
      if (cachedData === undefined && initialBuildInfoData !== undefined) {
        return initialBuildInfoData;
      }

      return API.getBuildInfo();
    },
  } satisfies UseQueryOptions<BuildInfoResponse>;
};
