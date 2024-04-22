import type { QueryClient, UseQueryOptions } from "react-query";
import * as API from "api/api";
import type { AppearanceConfig } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";

const initialAppearanceData = getMetadataAsJSON<AppearanceConfig>("appearance");
const appearanceConfigKey = ["appearance"] as const;

export const appearance = () => {
  const opts: UseQueryOptions<AppearanceConfig> = {
    queryKey: ["appearance"],
    initialData: initialAppearanceData,
    queryFn: () => API.getAppearance(),
  };
  // If we have initial appearance data, we don't want to fetch
  // the user again. We already have it!
  if (initialAppearanceData) {
    opts.cacheTime = Infinity;
    opts.staleTime = Infinity;
    opts.refetchOnMount = false;
    opts.refetchOnReconnect = false;
    opts.refetchOnWindowFocus = false;
  }
  return opts;
};

export const updateAppearance = (queryClient: QueryClient) => {
  return {
    mutationFn: API.updateAppearance,
    onSuccess: (newConfig: AppearanceConfig) => {
      queryClient.setQueryData(appearanceConfigKey, newConfig);
    },
  };
};
