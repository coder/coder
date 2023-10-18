import { QueryClient, type UseQueryOptions } from "react-query";
import * as API from "api/api";
import { type AppearanceConfig } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";

const initialAppearanceData = getMetadataAsJSON<AppearanceConfig>("appearance");
const appearanceConfigKey = ["appearance"] as const;

export const appearance = (queryClient: QueryClient) => {
  return {
    queryKey: appearanceConfigKey,
    queryFn: async () => {
      const cachedData = queryClient.getQueryData(appearanceConfigKey);
      if (cachedData === undefined && initialAppearanceData !== undefined) {
        return initialAppearanceData;
      }

      return API.getAppearance();
    },
  } satisfies UseQueryOptions<AppearanceConfig>;
};

export const updateAppearance = (queryClient: QueryClient) => {
  return {
    mutationFn: API.updateAppearance,
    onSuccess: (newConfig: AppearanceConfig) => {
      queryClient.setQueryData(appearanceConfigKey, newConfig);
    },
  };
};
