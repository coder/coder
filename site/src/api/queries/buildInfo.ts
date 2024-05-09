import { API } from "api/api";
import type { BuildInfoResponse } from "api/typesGenerated";
import type { MetadataState } from "hooks/useEmbeddedMetadata";
import { cachedQuery } from "./util";

const buildInfoKey = ["buildInfo"] as const;

export const buildInfo = (metadata: MetadataState<BuildInfoResponse>) => {
  // The version of the app can't change without reloading the page.
  return cachedQuery({
    metadata,
    queryKey: buildInfoKey,
    queryFn: () => API.getBuildInfo(),
  });
};
