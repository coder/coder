import * as API from "api/api";
import { BuildInfoResponse } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";

export const buildInfo = () => {
  return {
    queryKey: ["buildInfo"],
    queryFn: async () =>
      getMetadataAsJSON<BuildInfoResponse>("build-info") ?? API.getBuildInfo(),
  };
};
