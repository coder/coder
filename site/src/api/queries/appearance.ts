import { QueryClient } from "react-query";
import * as API from "api/api";
import { AppearanceConfig } from "api/typesGenerated";
import { getMetadataAsJSON } from "utils/metadata";

export const appearance = () => {
  return {
    queryKey: ["appearance"],
    queryFn: async () =>
      getMetadataAsJSON<AppearanceConfig>("appearance") ?? API.getAppearance(),
  };
};

export const updateAppearance = (queryClient: QueryClient) => {
  return {
    mutationFn: API.updateAppearance,
    onSuccess: (newConfig: AppearanceConfig) => {
      queryClient.setQueryData(["appearance"], newConfig);
    },
  };
};
