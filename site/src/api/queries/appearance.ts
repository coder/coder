import type { QueryClient } from "react-query";
import { client } from "api/api";
import type { AppearanceConfig } from "api/typesGenerated";
import type { MetadataState } from "hooks/useEmbeddedMetadata";
import { cachedQuery } from "./util";

const appearanceConfigKey = ["appearance"] as const;

export const appearance = (metadata: MetadataState<AppearanceConfig>) => {
  return cachedQuery({
    metadata,
    queryKey: ["appearance"],
    queryFn: () => client.api.getAppearance(),
  });
};

export const updateAppearance = (queryClient: QueryClient) => {
  return {
    mutationFn: client.api.updateAppearance,
    onSuccess: (newConfig: AppearanceConfig) => {
      queryClient.setQueryData(appearanceConfigKey, newConfig);
    },
  };
};
