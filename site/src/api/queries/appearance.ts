import type { QueryClient } from "react-query";
import * as API from "api/api";
import type { AppearanceConfig } from "api/typesGenerated";
import type { MetadataState } from "hooks/useEmbeddedMetadata";
import { cachedQuery } from "./util";

export const appearanceConfigKey = ["appearance"] as const;

export const appearance = (metadata: MetadataState<AppearanceConfig>) => {
  return cachedQuery({
    metadata,
    queryKey: appearanceConfigKey,
    queryFn: () => API.getAppearance(),
  });
};

export const updateAppearance = (queryClient: QueryClient) => {
  return {
    mutationFn: API.updateAppearance,
    onSuccess: (newConfig: AppearanceConfig) => {
      queryClient.setQueryData(appearanceConfigKey, newConfig);
    },
  };
};
