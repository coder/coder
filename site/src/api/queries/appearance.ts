import type { QueryClient, UseQueryOptions } from "react-query";
import * as API from "api/api";
import type { AppearanceConfig } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";
import { cachedQuery } from "./util";

const initialAppearanceData = getMetadataAsJSON<AppearanceConfig>("appearance");
const appearanceConfigKey = ["appearance"] as const;

export const appearance = (): UseQueryOptions<AppearanceConfig> => {
  // We either have our initial data or should immediately fetch and never again!
  return cachedQuery({
    initialData: initialAppearanceData,
    queryKey: ["appearance"],
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
